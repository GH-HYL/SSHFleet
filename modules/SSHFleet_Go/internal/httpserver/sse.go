package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// WriteSSE 写入一条 SSE 数据
func WriteSSE(w http.ResponseWriter, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", jsonData); err != nil {
		return err
	}
	w.(http.Flusher).Flush()
	return nil
}
