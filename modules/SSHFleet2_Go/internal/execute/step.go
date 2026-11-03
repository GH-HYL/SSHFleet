package execute

// 连上之后按模式分派（建站 05 规格第五节）：每一支一个文件，**不给既有函数加
// 「这个模式还不支持」那种 if**。
//
// 建连、收尾、定分类仍由 `visit` 收口——那是建站 04 定的，不动。这里只做「建连之后
// 那一步」，并把「这一步失败了」变成一条错误交回去；写进纸上的事实由各支自己填。

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

// step 走这一步。交回非 nil 表示这一步没能开始或中途栽了，报错原文由调用方并进那一格。
func (c *client) step(ctx context.Context, p *contract.Plan, res *contract.NodeResult, ag *aggregator) error {
	switch p.Mode {
	case contract.ModeCommand, contract.ModeScript:
		// 命令与脚本走同一条路：差别全在下发行那一格（脚本正文走 stdin、$0 补名字），
		// 执行站在这里看不出区别——那正是「拼装搬回解析站」带来的形态（02 号单）。
		c.runCommand(ctx, p, res)
		return nil
	case contract.ModeUpload:
		c.upload(ctx, p, res, progressCb(ag))
		return nil
	case contract.ModeDownload:
		c.download(ctx, p, res, progressCb(ag))
		return nil
	case contract.ModePasswd:
		c.passwd(ctx, p, res)
		return nil
	}
	return nil
}
