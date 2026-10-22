# 取数：DSH 会话记录

本附件只讲**怎么找到会话、哪些行算用户原话**。判据与产出回 `SKILL.md`。

## 一、会话在哪

`$USERPROFILE/.dsh/sessions/<工作区转写目录>/<会话目录>/session.v4.jsonl.zstd`

- **工作区转写目录**：工作区绝对路径，盘符后的 `:` 与 `\` 都换成 `-`，两侧补 `--`。`D:\work\p` → `--D-work-p--`
- **会话目录**：主会话 `session-<uuid>`；子 agent 是裸 `<uuid>`
- 三个易错点：盘符后的冒号要先吃掉（否则 `D--work`）；连续分隔符压成一个；首尾 `--` 是补的

目录不在就模糊匹配：`ls -1 ~/.dsh/sessions | grep -i SSHFleet`

**只看 `delegationDepth` 为 0 的**——那才是主会话，`depth1` 以上是子 agent。同工作区同时段可能有多条，按用户给的时间点或标题定位，别默认取最新。

## 二、怎么把文件读出来

文件是**多帧 zstd 拼接**，`zstd -dc` 只解第一帧，必须按 magic（`28 b5 2f fd`）切帧：

```bash
mkdir -p build/review
node -e '
const z=require("zlib"),fs=require("fs");
const M=Buffer.from([0x28,0xb5,0x2f,0xfd]);
const b=fs.readFileSync(process.argv[1]);const ix=[];let i=0;
while((i=b.indexOf(M,i))!==-1){ix.push(i);i++;}
let o="";for(let k=0;k<ix.length;k++){const s=ix[k],e=k+1<ix.length?ix[k+1]:b.length;
 try{o+=z.zstdDecompressSync(b.subarray(s,e)).toString("utf8")}catch(e){}}
process.stdout.write(o);
' "$ROOT/$T/session-<uuid>/session.v4.jsonl.zstd" > build/review/session-export.jsonl
```

产物落**工作区内**的 `build/`。

## 三、哪些行算用户原话

**`type === "user/message"` 且 `m.data.source.kind === "user"`。** 其余都不是：`system-reminder`、`runtime-context`、`time-context`，**以及子 agent 的回报**——那些也是 `user/message` 形态，但那是助手派出去的。

```bash
node -e '
const fs=require("fs");
const ls=fs.readFileSync("build/review/session-export.jsonl","utf8").split("\n").filter(Boolean);
let n=0;
for(const l of ls){try{const m=JSON.parse(l);
 if(m.type!=="user/message")continue;
 if(!(m.data&&m.data.source&&m.data.source.kind==="user"))continue;
 const c=m.data&&m.data.content;
 const txt=Array.isArray(c)?c.map(x=>x.text||"").join(""):String(c||"");
 n++;console.log("### U"+n+" ###\n"+txt.slice(0,1500)+"\n");
}catch(e){}}
'
```

**把 U1…Un 完整读一遍再动笔**，并当作引用编号。

**三个坑**：写入是**边跑边追加**，最后几步可能缺席；文件往往不小，**分段读**；子 agent 的会话不算依据。
