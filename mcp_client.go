package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

type MCPClient struct {
	Ctx    context.Context
	Client *client.Client
	Tools  []mcp.Tool
	Cmd    string
	Args   []string
	Env    []string
}

func NewMCPClient(ctx context.Context, cmd string, env, args []string) *MCPClient {
	stdioTransport := transport.NewStdio(cmd, env, args...)
	cli := client.NewClient(stdioTransport)
	m := &MCPClient{
		Ctx:    ctx,
		Client: cli,
		Cmd:    cmd,
		Args:   args,
		Env:    env,
	}
	return m
}

func (m *MCPClient) Start() error {
	err := m.Client.Start(m.Ctx)
	if err != nil {
		return err
	}
	mcpInitReq := mcp.InitializeRequest{}
	mcpInitReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	mcpInitReq.Params.ClientInfo = mcp.Implementation{
		Name:    "example-client",
		Version: "0.0.1",
	}
	if _, err = m.Client.Initialize(m.Ctx, mcpInitReq); err != nil {
		fmt.Println("mcp init error:", err)
		return err
	}
	return err
}

func (m *MCPClient) SetTools() error {
	toolsReq := mcp.ListToolsRequest{}
	tools, err := m.Client.ListTools(m.Ctx, toolsReq)
	if err != nil {
		return err
	}
	mt := make([]mcp.Tool, 0)
	for _, tool := range tools.Tools {
		mt = append(mt, mcp.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	m.Tools = mt
	return nil
}

func (m *MCPClient) CallTool(name string, args any) (string, error) {
	var arguments map[string]any
	switch v := args.(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &arguments); err != nil {
			return "", err
		}
	case map[string]any:
		arguments = v
	default:
	}
	res, err := m.Client.CallTool(m.Ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: arguments,
		},
	})
	if err != nil {
		return "", err
	}
	return mcp.GetTextFromContent(res.Content), nil
}

func (m *MCPClient) Close() {
	_ = m.Client.Close()
}

func (m *MCPClient) GetTool() []mcp.Tool {
	return m.Tools
}
