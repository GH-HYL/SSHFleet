# DSH 会话记录

DSH 的会话记录存在本机，需要时直接读。

**位置**：`~/.dsh/sessions/<工作区转写目录>/<会话目录>/session.v4.jsonl.zstd`

- **工作区转写目录**：工作区绝对路径，`/`、`\`、`:` 一律换成 `-`，连续的分隔符压成一个，两头各补一个 `-`（Windows 上盘符带冒号，所以成 `--D-work-p--` 这样）。`D:\work\p` → `--D-work-p--`
- **会话目录**：主会话 `session-<uuid>`，子 agent 是裸 `<uuid>`。
- **核对目录**：首条 `type:"session"` 记录里有 `cwd`、`createdAt`、`delegationDepth`；`cwd` 对得上工作区（`delegationDepth` 为 0），才算找对了，别只看目录名。

目录算出来不在，先回头核对公式，再模糊匹配：

```bash
ls -1 ~/.dsh/sessions | grep -i 关键字
```

**找哪一条**：多半只给会话标题，也可能什么线索都没有；别默认只查当前工作区那个目录。

- **给了标题**：标题在压缩帧里，`grep` 扫不到，只能逐个解压后读 `session/title` 的 `data.title`；同一会话可能记多条，取最后一条。
- **没给线索**：同一工作区目录里按最后一条 `user/message` 的 `time`（epoch 毫秒）取最新，跳过只有 `session`、`permission/preset` 这类记录的空壳会话。

**读的时候**：文件是**多帧 zstd 拼接**，整段一次解只出第一帧，得按 magic `28 b5 2f fd` 切帧逐帧解（本机不一定有 `zstd` 命令）。

用户原话是 `type=="user/message"` 且 `m.data.source.kind=="user"`，正文在 `m.data.content`（数组时取 `text` 拼接）。`system-reminder`、`runtime-context`、`time-context` 也是 `user/message` 形态，不算。

**子 agent 先排掉**：子 agent 目录里那条 `user/message` 的 `source.kind` 同样是 `user`，内容是派活的提示词；只靠 kind 过滤会把它当用户原话。

**弹窗问答不在 `user/message` 里**：`ask_user_question` 的回答落在 `type=="tool/result"` 的 `data.message.content[0].text`，是个 JSON 字符串（要再解一层），形如 `{"answers":[{"id":…,"selected":["A（推荐）"]}]}`；手打时 `selected` 为空、答案在 `custom`。题干不在里面，得按 `callId` 回同一条 `tool/call` 的 `arguments` 里找。
