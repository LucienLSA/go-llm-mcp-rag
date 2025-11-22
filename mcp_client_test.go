package main

import (
	"context"
	"fmt"
	"testing"
)

func TestMCPClient(t *testing.T) {
	ctx := context.Background()
	mcpCli := NewMCPClient(ctx, "uvx", nil, []string{"mcp-server-fetch"})
	err := mcpCli.Start()
	if err != nil {
		fmt.Println("start err", err)
		return
	}
	err = mcpCli.SetTools()
	if err != nil {
		fmt.Println("set tools err", err)
		return
	}
	tools := mcpCli.GetTool()
	fmt.Println(tools)
}
