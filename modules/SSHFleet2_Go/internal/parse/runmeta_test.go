package parse

import (
	"encoding/base64"
	"testing"

	"sshfleet2/internal/contract"
)

// runMetaVector 是同一份内容在**两处代码下**必须产出同一串字节的证据：
// 封这一侧拿它比「封出来是不是这串」，解那一侧（internal/report/runmeta_test.go）
// 拿它比「这串能不能开回同一份内容」。谁把密钥或格式写歪了，两边各有一条会红。
//
// 期望值不是被测代码现算的：这串是格式定稿后录下来的固定样本。
const runMetaVector = "AfdssCUJKsojw9X630aRo22ZZgb/+0pRd99AmUWcfb4Nr+VFTGg1g1UhR9xO6+gbmfvBV2VQlcpfdt6zd/o2YPZSjTJLhWw+4UE872GTOc6oeu3FTRPY6UBxbAf06KibkiVeV2QVXEzKXeL2XZb6qJS0/lx7qs29nkGrMR1iqn1c2Sd6HILvMnpx7kjBca06HnV5Y0qCCjNCt4fKDPPNtw=="

func runMetaSample() contract.RunMetaBody {
	return contract.RunMetaBody{
		Argv:           []string{"sshfleet2", "-f", "nodes.csv", "-c", "uptime"},
		Mode:           contract.ModeCommand,
		Command:        "uptime",
		Concurrency:    0,
		TimeoutConnect: 10,
		TimeoutExecute: 60,
	}
}

func TestSealRunMetaMatchesRecordedVector(t *testing.T) {
	sealed, err := sealRunMeta(runMetaSample())
	if err != nil {
		t.Fatalf("封不起来：%v", err)
	}
	if got := base64.StdEncoding.EncodeToString(sealed); got != runMetaVector {
		t.Fatalf("封出来的字节与录下的样本对不上\n got=%s\nwant=%s", got, runMetaVector)
	}
}

// 同一份内容封两次必须一字不差——nonce 取明文哈希，就是为了这条（见 runmeta.go）。
func TestSealRunMetaIsStable(t *testing.T) {
	a, err := sealRunMeta(runMetaSample())
	if err != nil {
		t.Fatalf("封不起来：%v", err)
	}
	b, err := sealRunMeta(runMetaSample())
	if err != nil {
		t.Fatalf("封不起来：%v", err)
	}
	if string(a) != string(b) {
		t.Fatal("同一份原值封出来两次不一样，两处的固定样本就没法对账了")
	}
}

// 明文不许出现在那段字节里——封的意义就在这一条。
func TestSealRunMetaHidesPlaintext(t *testing.T) {
	sealed, err := sealRunMeta(runMetaSample())
	if err != nil {
		t.Fatalf("封不起来：%v", err)
	}
	if contains(sealed, []byte("uptime")) {
		t.Fatal("那段字节里能直接找到命令原文，等于没封")
	}
}

func contains(hay, needle []byte) bool {
	if len(needle) == 0 || len(hay) < len(needle) {
		return false
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		same := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	return false
}

// 那一格收的是「使用者敲了什么」：整条命令行、模式、这一模式的原值、生效的执行参数。
// 它不重算——执行参数取纸上算好的那几个数，不是再解一遍命令行。
func TestRunMetaBodyCarriesOriginals(t *testing.T) {
	plan := &contract.Plan{
		Mode: contract.ModeCommand,
		Task: contract.Task{Command: &contract.CommandTask{Text: "uptime"}, Path: "/tmp/x"},
		Run: contract.RunParams{
			Concurrency: 4, TimeoutConnect: 10, TimeoutExecute: 60,
			TimeoutTransfer: 300, Answer: "y,continue",
		},
	}
	argv := []string{"sshfleet2", "-f", "nodes.csv", "-c", "uptime"}

	body := runMetaBodyOf(plan, argv)

	if len(body.Argv) != len(argv) || body.Argv[4] != "uptime" {
		t.Fatalf("整条命令行没原样装进去：%v", body.Argv)
	}
	if body.Mode != contract.ModeCommand || body.Command != "uptime" {
		t.Fatalf("模式或命令原文不对：%+v", body)
	}
	if body.Concurrency != 4 || body.TimeoutExecute != 60 || body.Answers != "y,continue" {
		t.Fatalf("执行参数没带上：%+v", body)
	}
	// 路径不是命令模式的东西，不该被顺手装进来。
	if body.Target != "" {
		t.Fatalf("命令模式不该带目标位置：%q", body.Target)
	}
}

// 装进去的是副本：改了 argv 不该影响纸上已经封好的那份。
func TestRunMetaBodyCopiesArgv(t *testing.T) {
	argv := []string{"sshfleet2", "-c", "uptime"}
	body := runMetaBodyOf(&contract.Plan{Task: contract.Task{Command: &contract.CommandTask{Text: "uptime"}}}, argv)
	argv[2] = "whoami"
	if body.Argv[2] != "uptime" {
		t.Fatal("那一格与调用方的切片共用了底层数组，改一边会动另一边")
	}
}
