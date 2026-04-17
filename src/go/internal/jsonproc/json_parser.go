package jsonproc

import (
	"encoding/json"
	"fmt"
	"sshexec_go/internal/ssh"

	"github.com/fatih/color"
)

// ParseRequestJSON 解析JSON请求数据
func ParseRequestJSON(requestJSON string) *RequestData {
	var jsonReq struct {
		CommandArgs   *CommandArgs        `json:"args"`
		Config        *Config             `json:"config"`
		Nodesinfo     []*NodeInfo         `json:"nodesinfo"`
		MonitorInfo   string              `json:"monitor_info"`
		ExecLogDir    string              `json:"exec_log_dir"`
		FinalCommand  string              `json:"final_command"`
		ErrorKeywords map[string][]string `json:"error_keywords"`
	}

	if err := json.Unmarshal([]byte(requestJSON), &jsonReq); err != nil {
		msg := fmt.Sprintf("JSON请求反序列化失败: %v", err)
		color.Red(msg)
		panic(err)
	}
	// 赋值错误分类变量
	ssh.ErrorKeywords = jsonReq.ErrorKeywords

	//// 解析错误关键词
	//err := json.Unmarshal([]byte(jsonReq.ErrorKeywords), &ssh.ErrorKeywords)
	//if err != nil {
	//	msg := fmt.Sprintf("错误分类关键词反序列化失败: %v", err)
	//	color.Red(msg)
	//	panic(err)
	//}

	return &RequestData{
		CommandArgs:   jsonReq.CommandArgs,
		Config:        jsonReq.Config,
		NodesInfo:     jsonReq.Nodesinfo,
		MonitorInfo:   jsonReq.MonitorInfo,
		ExecLogDir:    jsonReq.ExecLogDir,
		FinalCommand:  jsonReq.FinalCommand,
		ErrorKeywords: jsonReq.ErrorKeywords,
	}
}

//// 处理执行结果并构建响应
//func ProcessResults(results []*ssh.ExecResult, finalCommand string) string {
//	type ResponseData struct {
//		Command string        `json:"command"`
//		Success bool          `json:"success"`
//		Error   string        `json:"error,omitempty"`
//		Results []interface{} `json:"results"`
//	}
//
//	response := ResponseData{
//		Command: finalCommand,
//		Success: true,
//		Results: make([]interface{}, 0, len(results)),
//	}
//
//	for _, result := range results {
//		response.Results = append(response.Results, result)
//	}
//
//	responseJSON, err := json.MarshalIndent(response, "", "    ")
//	if err != nil {
//		msg := fmt.Sprintf("结果序列化失败，错误类型: %T", err)
//		log.Zlog.Error(msg)
//		color.Red(msg)
//		panic(err)
//	}
//	return string(responseJSON)
//}
