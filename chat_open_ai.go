package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	ark "github.com/sashabaranov/go-openai"
)

type ChatOpenAI struct {
	Ctx          context.Context
	Model        string
	SystemPrompt string
	Tools        []mcp.Tool
	RagContext   string
	Message      []ark.ChatCompletionMessage
	LLM          *ark.Client
}

type LLMOption func(*ChatOpenAI)

func WithSystemPrompt(prompt string) LLMOption {
	return func(ai *ChatOpenAI) {
		ai.SystemPrompt = prompt
	}
}

func WithRagContext(ragPrompt string) LLMOption {
	return func(ai *ChatOpenAI) {
		ai.RagContext = ragPrompt
	}
}

func WithMessage(message []ark.ChatCompletionMessage) LLMOption {
	return func(ai *ChatOpenAI) {
		ai.Message = message
	}
}

func WithLLMTools(tools []mcp.Tool) LLMOption {
	return func(ai *ChatOpenAI) {
		ai.Tools = tools
	}
}

func NewChatOpenAI(ctx context.Context, model string, opts ...LLMOption) *ChatOpenAI {
	if model == "" {
		panic("model is required")
	}
	var (
		apiKey  = os.Getenv(ArkAPIKeyEnv)
		baseURL = os.Getenv(ArkBaseURLEnv)
	)
	if apiKey == "" {
		panic("missing ARK_API_KEY")
	}
	if baseURL != "" {
		fmt.Println("use custom ark base url:", baseURL)
	} else {
		baseURL = "https://ark.cn-beijing.volces.com/api/v3"
	}
	config := ark.DefaultConfig(apiKey)
	config.BaseURL = baseURL
	cli := ark.NewClientWithConfig(config)
	llm := &ChatOpenAI{
		Ctx:     ctx,
		Model:   model,
		LLM:     cli,
		Message: make([]ark.ChatCompletionMessage, 0),
	}
	for _, opt := range opts {
		opt(llm)
	}
	if llm.SystemPrompt != "" {
		llm.Message = append(llm.Message, ark.ChatCompletionMessage{
			Role:    ark.ChatMessageRoleSystem,
			Content: llm.SystemPrompt,
		})
	}
	if llm.RagContext != "" {
		llm.Message = append(llm.Message, ark.ChatCompletionMessage{
			Role:    ark.ChatMessageRoleUser,
			Content: llm.RagContext,
		})
	}
	fmt.Println("init LLM successfully")
	return llm
}

func (c *ChatOpenAI) Chat(prompt string) (string, []ToolCall) {
	fmt.Println("init chat...")
	if prompt != "" {
		c.Message = append(c.Message, ark.ChatCompletionMessage{
			Role:    ark.ChatMessageRoleUser,
			Content: prompt,
		})
	}
	req := ark.ChatCompletionRequest{
		Model:    c.Model,
		Messages: c.Message,
		Tools:    MCPTool2ArkTool(c.Tools),
	}
	resp, err := c.LLM.CreateChatCompletion(c.Ctx, req)
	if err != nil {
		panic(err)
	}
	if len(resp.Choices) == 0 {
		return "", nil
	}
	assistantMsg := resp.Choices[0].Message
	c.Message = append(c.Message, assistantMsg)
	return assistantMsg.Content, ArkToolCallsToInternal(assistantMsg.ToolCalls)
}

func MCPTool2ArkTool(mcpTools []mcp.Tool) []ark.Tool {
	if len(mcpTools) == 0 {
		return nil
	}
	openAITools := make([]ark.Tool, 0, len(mcpTools))
	for _, tool := range mcpTools {
		params := map[string]any{
			"type":       tool.InputSchema.Type,
			"properties": tool.InputSchema.Properties,
			"required":   tool.InputSchema.Required,
		}
		if t, ok := params["type"].(string); !ok || t == "" {
			params["type"] = "object"
		}
		openAITools = append(openAITools, ark.Tool{
			Type: ark.ToolTypeFunction,
			Function: &ark.FunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  params,
			},
		})
	}
	return openAITools
}

type ToolCall struct {
	ID       string
	Function ToolFunction
}

type ToolFunction struct {
	Name      string
	Arguments string
}

func ArkToolCallsToInternal(calls []ark.ToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		result = append(result, ToolCall{
			ID: call.ID,
			Function: ToolFunction{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	return result
}
