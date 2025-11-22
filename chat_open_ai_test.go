package main

import (
	"context"
	"fmt"
	"testing"
)

func TestChatOpenAI_Chat(t *testing.T) {
	ctx := context.Background()
	model := "doubao-seed-1-6-251015"
	ai := NewChatOpenAI(ctx, model, WithRagContext(""), WithSystemPrompt(""))
	prompt := "hello!"
	result, toolCalls := ai.Chat(prompt)
	fmt.Println("result", result, "toolCalls", toolCalls)
}
