# 取数：DSH 会话记录

本附件只讲**怎么把 DSH 的会话记录找出来、怎么判断哪些行是用户原话**。判据与产出回 `SKILL.md`。

## 一、定位会话记录

```
<DSH 会话根>/<工作区转写目录>/<会话目录>/session.v4.jsonl.zstd
```

- 会话根：`$USERPROFILE/.dsh/sessions`（Windows）或 `~/.dsh/sessions`
- 工作区转写目录：工作区绝对路径，`\` 与 `:` 换成 `-`，两侧补 `--`。`D:\work\p` → `--D-work-p--`
- 会话目录：主会话 `session-<uuid>`；子 agent 是裸 `<uuid>`

**三个易错点**：盘符后的冒号要先吃掉（否则 `D--work`）；连续分隔符压成一个；首尾 `--` 是补的。

目录不存在就模糊匹配：`ls -1 "$ROOT" | grep -i "${WS##*/}"`

### 按标题找会话

```bash
node -e '
const z=require("zlib"),fs=require("fs");
const M=Buffer.from([0x28,0xb5,0x2f,0xfd]);
function dec(f){const b=fs.readFileSync(f);const ix=[];let i=0;
 while((i=b.indexOf(M,i))!==-1){ix.push(i);i++;}
 let o="";for(let k=0;k<ix.length;k++){const s=ix[k],e=k+1<ix.length?ix[k+1]:b.length;
  try{o+=z.zstdDecompressSync(b.subarray(s,e)).toString("utf8")}catch(e){}}return o;}
for(const d of fs.readdirSync(".")){const f=d+"/session.v4.jsonl.zstd";
 if(!fs.existsSync(f))continue;
 const t=dec(f);let title="";
 for(const ln of t.split("\n").slice(0,80)){try{const m=JSON.parse(ln);
  if(m.type==="session/title"&&m.data&&m.data.title)title=m.data.title;}catch(e){}}
 const m=JSON.parse(t.split("\n")[0]);
 console.log(d,"|",new Date(m.createdAt).toISOString(),"|d"+m.delegationDepth,"|",title);}
'
```

**`depth0` 才是主会话**；`depth1` 以上是子 agent。同工作区同时段可能有多条 `depth0`，按用户给的时间点或标题定位，别默认取最新。

### 导出

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

**三个坑**：文件是**多帧 zstd 拼接**（`zstd -dc` 只解第一帧，必须按 magic 切帧）；写入是**边跑边追加**，最后几步可能缺席；产物落**工作区内**的 `build/`。

## 二、只有用户原话是权威

**只有 `source.kind === "user"` 的 `user/message` 是用户原话。** 其余都不是：`system-reminder`、`runtime-context`、`time-context`，**以及子 agent 的回报**——那些也是 `user/message` 形态，但那是助手派出去的。

```bash
node -e '
const fs=require("fs");
const ls=fs.readFileSync("build/review/session-export.jsonl","utf8").split("\n").filter(Boolean);
let n=0;
for(const l of ls){try{const m=JSON.parse(l);
 if(m.type!=="user/message")continue;
 if(!(m.source&&m.source.kind==="user"))continue;
 const c=m.data&&m.data.content;
 const txt=Array.isArray(c)?c.map(x=>x.text||"").join(""):String(c||"");
 n++;console.log("### U"+n+" ###\n"+txt.slice(0,1500)+"\n");
}catch(e){}}
'
```

**把 U1…Un 完整读一遍再动笔**，并当作引用编号。长对话里真·原话的条数远少于消息总数。文件往往很大，**分段读，别一次塞进上下文**——那正好复制了本技能要防的毛病。
