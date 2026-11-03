package contract

// 代填（`-a`）那一件事的两种格子：一条代填本身，以及一台机器跑完之后的三条事实。
//
// 名称分两处放：表在「这次干什么」那一格里（命令与脚本载荷各一份），事实在「每台一行」
// 那一格里（`NodeResult.Answer`）。两者不是一个东西——表是输入，事实是结果。

// Answer 一条代填：填进远端输入通道的内容，以及认出提问的触发词。
//
// 顺序即命令行给出的顺序：执行站按序送、报告站按序印（序号从 1 起），
// 「第几条没送出去」这句话里的号指的就是这个顺序。
//
// Value 留空是合法写法（只发一个回车），呈现时写成「（回车）」——那归 message。
type Answer struct {
	Value    string   `json:"value"`
	Triggers []string `json:"triggers"`
}

// AnswerResult 一台机器上代填的收尾事实，三条各自成格。
//
// 这三条都**不是结论**：判「代填没送完 ⇒ 失败」是判定那一步的事，这里只记发生了什么。
type AnswerResult struct {
	// Missed 是没送出去的条目序号（1 起，与 `Answers` 的顺序对位）。
	Missed []int `json:"missed,omitempty"`

	// Exhausted 是「表已经送空、还在等」：工具分不出「脚本卡在没配触发词的提示上」
	// 与「任务本身耗时长」，但表空了是确定的事实。
	Exhausted bool `json:"exhausted,omitempty"`

	// AbortLine 是中止词命中的那一行原文；没命中为空。
	AbortLine string `json:"abort_line,omitempty"`
}
