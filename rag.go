package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// RAGRetriever 实现简单的 RAG 检索功能
type RAGRetriever struct {
	KnowledgeBaseDir string
	MaxResults       int
	MinScore         float64
}

// NewRAGRetriever 创建新的 RAG 检索器
func NewRAGRetriever(knowledgeBaseDir string) *RAGRetriever {
	return &RAGRetriever{
		KnowledgeBaseDir: knowledgeBaseDir,
		MaxResults:       3,   // 默认返回最多3个相关文档
		MinScore:         0.1, // 最低相关性分数
	}
}

// Document 表示一个文档
type Document struct {
	Path    string
	Content string
	Score   float64
}

// Retrieve 根据查询检索相关文档
func (r *RAGRetriever) Retrieve(ctx context.Context, query string) (string, error) {
	if r.KnowledgeBaseDir == "" {
		return "", nil
	}

	// 检查知识库目录是否存在
	if _, err := os.Stat(r.KnowledgeBaseDir); os.IsNotExist(err) {
		return "", fmt.Errorf("knowledge base directory does not exist: %s", r.KnowledgeBaseDir)
	}

	// 读取所有文档
	documents, err := r.loadDocuments(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to load documents: %w", err)
	}

	if len(documents) == 0 {
		return "", nil
	}

	// 计算相关性分数
	scoredDocs := r.scoreDocuments(query, documents)

	// 选择最相关的文档
	selectedDocs := r.selectTopDocuments(scoredDocs)

	if len(selectedDocs) == 0 {
		return "", nil
	}

	// 构建 RAG 上下文
	context := r.buildContext(selectedDocs, query)

	return context, nil
}

// loadDocuments 从知识库目录加载所有文档
func (r *RAGRetriever) loadDocuments(ctx context.Context) ([]Document, error) {
	var documents []Document

	err := filepath.Walk(r.KnowledgeBaseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 只处理文本文件
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".txt" && ext != ".md" && ext != ".go" {
			return nil
		}

		// 读取文件内容
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// 只处理非空文件
		if len(content) == 0 {
			return nil
		}

		documents = append(documents, Document{
			Path:    path,
			Content: string(content),
		})

		return nil
	})

	return documents, err
}

// scoreDocuments 计算文档与查询的相关性分数
func (r *RAGRetriever) scoreDocuments(query string, documents []Document) []Document {
	queryWords := r.tokenize(query)
	queryWordSet := make(map[string]bool)
	for _, word := range queryWords {
		queryWordSet[strings.ToLower(word)] = true
	}

	scoredDocs := make([]Document, 0, len(documents))
	for _, doc := range documents {
		score := r.calculateScore(queryWords, queryWordSet, doc.Content)
		if score >= r.MinScore {
			doc.Score = score
			scoredDocs = append(scoredDocs, doc)
		}
	}

	return scoredDocs
}

// calculateScore 计算单个文档的相关性分数
func (r *RAGRetriever) calculateScore(queryWords []string, queryWordSet map[string]bool, content string) float64 {
	contentWords := r.tokenize(content)
	contentWordSet := make(map[string]int)

	// 统计内容中的词频
	for _, word := range contentWords {
		contentWordSet[strings.ToLower(word)]++
	}

	// 计算匹配的查询词数量
	matchedCount := 0
	for _, word := range queryWords {
		lowerWord := strings.ToLower(word)
		if queryWordSet[lowerWord] && contentWordSet[lowerWord] > 0 {
			matchedCount++
		}
	}

	if len(queryWords) == 0 {
		return 0
	}

	// 基础分数：匹配词比例
	baseScore := float64(matchedCount) / float64(len(queryWords))

	// 加权分数：考虑词频
	freqScore := 0.0
	for word, count := range contentWordSet {
		if queryWordSet[word] {
			freqScore += float64(count)
		}
	}
	if len(contentWords) > 0 {
		freqScore = freqScore / float64(len(contentWords))
	}

	// 综合分数
	finalScore := baseScore*0.7 + freqScore*0.3

	return finalScore
}

// tokenize 将文本分词（简单实现）
func (r *RAGRetriever) tokenize(text string) []string {
	// 转换为小写并分割
	text = strings.ToLower(text)
	var words []string

	// 简单的分词：按空格和标点分割
	currentWord := ""
	for _, char := range text {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			currentWord += string(char)
		} else {
			if len(currentWord) > 0 {
				words = append(words, currentWord)
				currentWord = ""
			}
		}
	}
	if len(currentWord) > 0 {
		words = append(words, currentWord)
	}

	return words
}

// selectTopDocuments 选择分数最高的文档
func (r *RAGRetriever) selectTopDocuments(documents []Document) []Document {
	if len(documents) == 0 {
		return documents
	}

	// 按分数从高到低排序
	sort.Slice(documents, func(i, j int) bool {
		return documents[i].Score > documents[j].Score
	})

	// 选择前 N 个文档
	if len(documents) <= r.MaxResults {
		return documents
	}

	topDocs := make([]Document, 0, r.MaxResults)
	for i := 0; i < r.MaxResults; i++ {
		topDocs = append(topDocs, documents[i])
	}

	return topDocs
}

// buildContext 构建 RAG 上下文字符串
func (r *RAGRetriever) buildContext(documents []Document, query string) string {
	var builder strings.Builder

	builder.WriteString("=== 相关上下文信息（RAG检索） ===\n")
	builder.WriteString(fmt.Sprintf("查询: %s\n\n", query))
	builder.WriteString("检索到的相关内容：\n\n")

	for i, doc := range documents {
		builder.WriteString(fmt.Sprintf("--- 文档 %d (相关性: %.2f) ---\n", i+1, doc.Score))
		builder.WriteString(fmt.Sprintf("来源: %s\n", doc.Path))

		// 截取文档内容的前1000个字符（避免上下文过长）
		content := doc.Content
		if len(content) > 1000 {
			content = content[:1000] + "..."
		}
		builder.WriteString(content)
		builder.WriteString("\n\n")
	}

	builder.WriteString("=== 上下文信息结束 ===\n")
	builder.WriteString("请基于以上上下文信息回答用户的问题。\n")

	return builder.String()
}
