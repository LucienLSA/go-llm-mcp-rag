package main

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	ark "github.com/sashabaranov/go-openai"
)

type Agent struct {
	MCPClient    []*MCPClient
	LLM          *ChatOpenAI
	Model        string
	SystemPrompt string
	Ctx          context.Context
	RAGCtx       string
	RAGRetriever *RAGRetriever // RAG 检索器
}

func NewAgent(ctx context.Context, model string, mcpCli []*MCPClient, systemPrompt string, ragCtx string) *Agent {
	// 1. 激活所有的mcp client 拿到所有的tools
	tools := make([]mcp.Tool, 0)
	for _, item := range mcpCli {
		// 启动 stdio 传输
		err := item.Start()
		if err != nil {
			fmt.Println("mcp listen error:", err)
			continue
		}

		err = item.SetTools()
		if err != nil {
			fmt.Println("mcp set tools error:", err)
			continue
		}

		// 新增日志：打印该客户端的工具名称，便于确认工具是否正确注册
		for _, t := range item.GetTool() {
			fmt.Println("tool ready:", t.Name)
		}
		tools = append(tools, item.GetTool()...)
	}
	// 2. 激活并告诉llm有哪些tools
	llm := NewChatOpenAI(ctx, model, WithSystemPrompt(systemPrompt), WithLLMTools(tools), WithRagContext(ragCtx))
	fmt.Println("init LLM & Tools")
	return &Agent{
		MCPClient:    mcpCli,
		LLM:          llm,
		Model:        model,
		SystemPrompt: systemPrompt,
		RAGCtx:       ragCtx,
		RAGRetriever: nil,
	}
}

func (a *Agent) Close() {
	for _, mcpClient := range a.MCPClient {
		mcpClient.Close()
	}
	fmt.Println("all close")
}

// NewAgentWithRAG 创建带有 RAG 检索功能的 Agent
func NewAgentWithRAG(ctx context.Context, model string, mcpCli []*MCPClient, systemPrompt string, knowledgeBaseDir string) *Agent {
	agent := NewAgent(ctx, model, mcpCli, systemPrompt, "")
	if knowledgeBaseDir != "" {
		agent.RAGRetriever = NewRAGRetriever(knowledgeBaseDir)
		fmt.Println("RAG retriever initialized with knowledge base:", knowledgeBaseDir)
	}
	return agent
}

func (a *Agent) Invoke(prompt string) string {
	if a.LLM == nil {
		return ""
	}
	response, toolCalls := a.LLM.Chat(prompt)
	fmt.Println("toolCalls", toolCalls)
	for len(toolCalls) > 0 {
		fmt.Println("response", response)
		for _, toolCall := range toolCalls {
			for _, mcpClient := range a.MCPClient {
				for _, mcpTool := range mcpClient.GetTool() {
					if mcpTool.Name == toolCall.Function.Name {
						fmt.Println("tool use", toolCall.ID, toolCall.Function.Name, toolCall.Function.Arguments)
						toolText, err := mcpClient.CallTool(toolCall.Function.Name, toolCall.Function.Arguments)
						if err != nil {
							fmt.Println("call tool error:", err)
							continue
						}
						a.LLM.Message = append(a.LLM.Message, ark.ChatCompletionMessage{
							Role:       ark.ChatMessageRoleTool,
							Content:    toolText,
							ToolCallID: toolCall.ID,
						})
					}
				}
			}
		}
		// 二次对话（空 prompt 也会发起请求）
		response, toolCalls = a.LLM.Chat("")
	}
	a.Close()
	return response
}

// InvokeWithRAG 使用 RAG 检索功能执行任务
func (a *Agent) InvokeWithRAG(prompt string) string {
	if a.LLM == nil {
		return ""
	}

	// 如果配置了 RAG 检索器，先进行检索
	if a.RAGRetriever != nil {
		fmt.Println("Retrieving relevant context from knowledge base...")
		ragContext, err := a.RAGRetriever.Retrieve(a.Ctx, prompt)
		if err != nil {
			fmt.Printf("RAG retrieval error: %v\n", err)
		} else if ragContext != "" {
			fmt.Println("RAG context retrieved, injecting into LLM...")
			// 将 RAG 上下文注入到 LLM
			a.LLM.Message = append(a.LLM.Message, ark.ChatCompletionMessage{
				Role:    ark.ChatMessageRoleUser,
				Content: ragContext,
			})
		}
	}

	// 执行正常的调用流程
	return a.Invoke(prompt)
}
