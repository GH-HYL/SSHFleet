package guard

import (
	"fmt"
	"go/ast"
)

// 纸面白名单（ADR-0044，05 号单）：契约里裸 `string` 的格子只许装三类东西——事实、
// 类别常量、配置原文；一格一行登记，每行选一个类别。名单外的裸 `string` 即红。
//
// 为什么写在代码里、不进放行台账：台账 `Exception` 的粒度是「规则 ＋ 路径」，放行一格
// 等于放行那个文件里的这一类全部，挡不住「再塞一句」；写在代码里，加一格要动这份名单、
// 当场选类别——加一格事实字段几乎无痛，往纸里塞一句拼好的话则必须声明它是报文原文或
// 配置原文，声明错了下一次审计就露。
//
// 名单默认只对自己这个模块生效：样本模块（testdata）里那些同名类型不该被真名单量；与
// helperHomeModule 同一条理由。样本要量自己的契约，就经 `Target.Paper` 递一张小的进来。
const paperHomeModule = "sshfleet2"

// paperPath 是名单在模块里的位置：名单自己烂了，报错要指到它。
const paperPath = "internal/guard/paper.go"

// PaperKind 是这一格装的是哪一类东西。
type PaperKind string

const (
	// PaperFact 事实：数、布尔、枚举名、路径、IP、使用者敲的原值；系统或解析器给的原话也原样带进来。
	PaperFact PaperKind = "事实"
	// PaperConfig 配置原文：配置里抄上来的原值。
	PaperConfig PaperKind = "配置原文"
	// PaperCategory 类别常量：装的是某个类别常量的名字，不是一句拼好的话。
	PaperCategory PaperKind = "类别常量"
	// PaperReport 报文原文：远端给的原话，工具不拼也不裁。全工程只此三格，别处不许按它登记。
	PaperReport PaperKind = "报文原文"
)

// PaperField 是白名单的一行：哪个类型的哪一格，装的是哪一类。
type PaperField struct {
	Type  string
	Field string
	Kind  PaperKind
}

// paperFields 是纸面白名单本体，随契约逐格登记。
//
// 判据只看类型（`string` 与它的三种变体），不看字段名、不看字面量：「名字像句子」判不出来，
// 能判的是「这一格是裸 string 而名单上没有」——名字骗得过去，类别骗不过审计。
var paperFields = []PaperField{
	// 代填与它的收尾（answer.go）
	{"Answer", "Value", PaperFact},
	{"Answer", "Triggers", PaperFact},
	{"AnswerResult", "AbortLine", PaperReport},

	// 起不来那一屏（cannot_start.go）
	{"CannotStart", "Where", PaperFact},
	// Head 装的是 `message.HeadXxx` 那组类别常量的名字：是类别，不是句子。
	{"CannotStartReason", "Head", PaperCategory},
	{"CannotStartReason", "Place", PaperFact},
	{"CannotStartReason", "Value", PaperFact},
	// Origin 是系统或解析器给的原话，工具不加工；它不是使用者那一屏的正文（报文原文只算那三格）。
	{"CannotStartReason", "Origin", PaperFact},
	{"CannotStartRow", "IP", PaperFact},

	// 危险检测（danger.go）：命中的原文是事实，规则那四格是配置原文
	{"DangerMatch", "Content", PaperFact},
	{"DangerMatch", "RuleName", PaperConfig},
	{"DangerMatch", "Level", PaperConfig},
	{"DangerMatch", "Example", PaperConfig},
	{"DangerMatch", "Reason", PaperConfig},
	{"DangerReport", "ScriptPath", PaperFact},
	{"DangerVerdict", "Highest", PaperFact},

	// 密钥管理（key.go）：路径与指纹都是事实
	{"KeyRun", "Path", PaperFact},
	{"KeyFacts", "EnvName", PaperFact},
	{"KeyFacts", "EnvPrint", PaperFact},
	{"KeyFacts", "PersistPrint", PaperFact},
	{"KeyFacts", "PersistWhere", PaperFact},
	{"KeyFacts", "OtherPrint", PaperFact},
	{"KeyFacts", "OtherWhere", PaperFact},
	{"KeyFacts", "RCPath", PaperFact},
	{"PersistResult", "Where", PaperFact},
	{"PersistResult", "OtherUnified", PaperFact},

	// 总账（ledger.go）
	{"CategoryCount", "Category", PaperFact},
	{"CategoryIPs", "Category", PaperFact},
	{"CategoryIPs", "IPs", PaperFact},
	{"Ledger", "Verify", PaperFact},
	{"Ledger", "SuccessCategory", PaperFact},
	{"Ledger", "SuccessIPs", PaperFact},

	// 每台机器的连接要素（node.go）
	{"Node", "IP", PaperFact},
	{"Node", "Port", PaperFact},
	{"Node", "User", PaperFact},
	{"Node", "Password", PaperFact},
	{"Node", "Key", PaperFact},
	{"Node", "KeyPassword", PaperFact},

	// 执行参数与解析时的问题（plan.go）
	{"RunParams", "Answer", PaperFact},
	{"RunParams", "Remark", PaperFact},
	{"RunParams", "FallbackExt", PaperConfig},
	{"RunParams", "Interpreter", PaperConfig},
	{"RunParams", "Env", PaperConfig},
	// Downlink 是解析站拼好的下发行：拼装只此一处（ADR-0010），它不是给使用者的话。
	{"RunParams", "Downlink", PaperFact},
	{"RunParams", "Stdin", PaperFact},
	{"Problem", "Option", PaperFact},
	{"Problem", "Raw", PaperFact},

	// 执行资源（resources.go）
	{"Resources", "Manifest", PaperFact},

	// 每台一行（result.go）
	{"NodeResult", "IP", PaperFact},
	{"NodeResult", "Port", PaperFact},
	{"NodeResult", "User", PaperFact},
	{"NodeResult", "Category", PaperFact},
	{"NodeResult", "ErrorText", PaperReport},
	{"NodeResult", "Output", PaperReport},
	{"NodeResult", "AuthMethod", PaperFact},

	// 运行上下文（runmeta.go）
	{"RunMetaBody", "Argv", PaperFact},
	{"RunMetaBody", "Command", PaperFact},
	{"RunMetaBody", "ScriptPath", PaperFact},
	{"RunMetaBody", "Source", PaperFact},
	{"RunMetaBody", "Target", PaperFact},
	{"RunMetaBody", "NewPassword", PaperFact},
	{"RunMetaBody", "ErrorKeywordsPath", PaperConfig},
	{"RunMetaBody", "AnswerRaw", PaperFact},

	// 中间步骤（step.go）：装的是 StepPrecheck 这类语义名，不是句子
	{"Step", "Name", PaperCategory},

	// 判据表（tables.go）：整张表都是从配置里读上来的
	{"DangerRule", "Name", PaperConfig},
	{"DangerRule", "Level", PaperConfig},
	{"DangerRule", "Example", PaperConfig},
	{"DangerRule", "Reason", PaperConfig},
	{"Category", "Name", PaperConfig},
	{"Category", "Keywords", PaperConfig},
	{"Category", "Tip", PaperConfig},
	{"PasswdStep", "Keywords", PaperConfig},
	{"PasswdStep", "Value", PaperConfig},

	// 这次干什么（task.go）
	{"Task", "Path", PaperFact},
	{"CommandTask", "Text", PaperFact},
	{"ScriptTask", "Path", PaperFact},
	{"ScriptTask", "Body", PaperFact},
	{"UploadTask", "Source", PaperFact},
	{"UploadTask", "Skipped", PaperFact},
	{"DownloadTask", "RemotePath", PaperFact},
	{"PasswdTask", "Raw", PaperFact},
	{"PasswdTask", "NewPassword", PaperFact},

	// 上传与传输（upload.go）
	{"UploadFile", "Path", PaperFact},
	{"UploadFile", "Rel", PaperFact},
	{"UploadEntry", "Name", PaperFact},
}

// checkPaperFields 判契约包的裸 `string` 格子是不是都在名单上，反过来也判名单有没有空行。
//
// 两个方向都报：只判「名单外的多出来」会让名单越攒越松（字段删了、改名了、收成具名类型了，
// 那一行还留着），松到最后谁也不知道哪一行还算数——与放行台账「没人用还在」同一条理由。
func checkPaperFields(m *Module) []Violation {
	fields := m.Paper
	if fields == nil {
		if m.ModulePath != paperHomeModule {
			return nil
		}
		fields = paperFields
	}
	p := m.PkgOfKind(KindContract)
	if p == nil {
		return nil
	}

	allowed := map[string]PaperKind{}
	for _, f := range fields {
		allowed[f.Type+"."+f.Field] = f.Kind
	}

	var out []Violation
	seen := map[string]bool{}
	for _, file := range p.Files {
		for _, decl := range file.AST.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, field := range st.Fields.List {
					if !bareString(field.Type) {
						continue
					}
					for _, name := range field.Names {
						key := ts.Name.Name + "." + name.Name
						seen[key] = true
						if _, ok := allowed[key]; ok {
							continue
						}
						out = append(out, file.violation(ClassContract, name.Pos(),
							fmt.Sprintf("契约里 %s 是一格裸 string，白名单上没有它", key),
							"纸只许装事实、类别常量与配置原文（ADR-0044）：一格自由字符串就是「站里自己拼一句、塞进纸」的那条暗道，留着它，绕开结构的人只是多写一行",
							"改成类别常量或具名类型；确有理由就把它写进 internal/guard/paper.go 的 paperFields，并为它选一个类别——加一格要动检查器代码，diff 里看得见"))
					}
				}
			}
		}
	}

	for _, f := range fields {
		key := f.Type + "." + f.Field
		if seen[key] {
			continue
		}
		out = append(out, violationAt(ClassContract, paperPath,
			fmt.Sprintf("白名单里的 %s 在契约里已经没有了", key),
			"这一行的意思是「那一格允许是裸 string」：字段被删、被改名，或者收成了具名类型之后还留着，名单就会越攒越松，下一次没人知道哪一行还算数",
			"把这一行删掉——删它同样是改白名单，与那一处改动同一次提交里做完"))
	}
	return out
}

// bareString 判一格是不是裸 string：`string`、`[]string`、`*string`、`map[string]string`。
//
// 具名类型（`Mode`、`CannotStartKind`、`ProblemSubject`…）不算：ADR-0044 要走的就是把它们
// 收成具名类型，判据得认得出这一步，否则收完了还红，这条路就没人走。
func bareString(t ast.Expr) bool {
	switch x := t.(type) {
	case *ast.Ident:
		return x.Name == "string"
	case *ast.ArrayType:
		return x.Len == nil && bareString(x.Elt)
	case *ast.StarExpr:
		return bareString(x.X)
	case *ast.MapType:
		return bareString(x.Key) && bareString(x.Value)
	}
	return false
}
