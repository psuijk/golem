package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

func main() {
	body := map[string]interface{}{
		"model":  "qwen3:30b",
		"stream": true,
		"messages": []map[string]string{
			{"role": "user", "content": "create a file called hello.txt with the content 'hello world'"},
		},
		"tools": []map[string]interface{}{
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "writefile",
					"description": "Write content to a file",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"path":    map[string]string{"type": "string"},
							"content": map[string]string{"type": "string"},
						},
						"required": []string{"path", "content"},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(body)

	resp, err := http.Post("http://localhost:11434/api/chat", "application/json", bytes.NewReader(data))
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
}
