package execute

// 连上之后按模式分派（建站 05 规格第五节）：每一支一个文件，**不给既有函数加
// 「这个模式还不支持」那种 if**。
//
// 建连、收尾、定分类仍由 `visit` 收口——那是建站 04 定的，不动。这里只做「建连之后
// 那一步」：成败与失败原文由各支自己并进纸上的那一格（ADR-0021），不从这里交回去。

import (
	"context"

	"sshfleet2/internal/contract"
)

// progressCb 节点侧的进度回调；聚合器不在（非传输路径）时给 nil。
func progressCb(ag *aggregator) onProgressN {
	if ag == nil {
		return nil
	}
	return ag.onProgress
}

// step 走这一步。这一步的成败不从这里交回：每一支都已经把失败原文并进 `res` 那一格
// （ADR-0021），所以没有 error 可交——签名里留一个永远是 nil 的返回值，等于给调用方
// 许诺一件不会发生的事。
func (c *client) step(ctx context.Context, p *contract.Plan, res *contract.NodeResult, ag *aggregator) {
	switch p.Mode {
	case contract.ModeCommand, contract.ModeScript:
		// 命令与脚本走同一条路：差别全在下发行那一格（脚本正文走 stdin、$0 补名字），
		// 执行站在这里看不出区别——那正是「拼装搬回解析站」带来的形态（02 号单）。
		c.runCommand(ctx, p, res)
	case contract.ModeUpload:
		c.upload(ctx, p, res, progressCb(ag))
	case contract.ModeDownload:
		c.download(ctx, p, res, progressCb(ag))
	case contract.ModePasswd:
		c.passwd(ctx, p, res)
	}
}
