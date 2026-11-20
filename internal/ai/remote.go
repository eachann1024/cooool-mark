package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/egoist/mygo"

	"cooool-mark/internal/app"
	"cooool-mark/internal/model"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func polishOpenAI(ctx context.Context, s model.Settings, in app.PolishInput) (app.Polish, error) {
	baseURL := strings.TrimSpace(s.OpenAIURL)
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	modelName := strings.TrimSpace(s.OpenAIModel)
	if modelName == "" {
		return app.Polish{}, errors.New("未设置 OpenAI 模型名称")
	}

	var apiKey string
	if rawKey, err := mygo.Secrets.Get("openai-key"); err == nil {
		apiKey = strings.TrimSpace(string(rawKey))
	}

	return polishOpenAICompat(ctx, s, baseURL, apiKey, modelName, in, nil)
}

func polishOpenAICompat(
	ctx context.Context,
	s model.Settings,
	baseURL, apiKey, modelName string,
	in app.PolishInput,
	extraFields map[string]any,
) (app.Polish, error) {
	endpoint := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}

	inst, prompt := buildPolishPrompt(in)
	payload := map[string]any{
		"model": modelName,
		"messages": []chatMessage{
			{Role: "system", Content: inst},
			{Role: "user", Content: prompt},
		},
		"temperature": 0.2,
		"max_tokens":  350,
	}
	for k, v := range extraFields {
		payload[k] = v
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return app.Polish{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return app.Polish{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := newHTTPClient(s, 0)
	resp, err := client.Do(req)
	if err != nil {
		return app.Polish{}, fmt.Errorf("请求模型接口失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return app.Polish{}, err
	}
	if resp.StatusCode == http.StatusUnauthorized && apiKey == "" {
		return app.Polish{}, errors.New("未配置 OpenAI API Key")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return app.Polish{}, fmt.Errorf("模型接口返回状态 %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}

	var parsed struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return app.Polish{}, fmt.Errorf("解析模型响应失败: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return app.Polish{}, errors.New(parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return app.Polish{}, errors.New("模型未返回任何结果")
	}
	return parsePolishJSON(parsed.Choices[0].Message.Content, in.Choices)
}
