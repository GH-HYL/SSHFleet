package report

import (
	"encoding/base64"
	"reflect"
	"testing"

	"sshfleet2/internal/contract"
)

// 这一串由 internal/parse 封出来（那边的 runmeta_test.go 拿同一串比「封出来是不是它」）。
// 这一侧比的是「这串能不能开回同一份内容」——两处密钥或格式一漂，必有一侧红。
const runMetaVector = "ARMJAw/HpBLHcCrKV+2Ort1bx2JVFzHoxzPxvcAsHDbuZWaYygYM7daO1PKY7qCpyIT5bzmGnwt8Woj7tb1dA5PiQBnDbZtYcnR0kCJk8PX+26NanE6Kczq0MhAXBntIdg16zo4PqUV0LYkDKGGF+fLyrkJJqrp1EMPCHJnBhLn1DZ0g3tMeT/w0VX1F94HhOAqujA/lIczVMbQYhVaxlXrB1RKxSGvjDy37h1+jaRqEsc7foSuIaimHV3chsbydVbgmmxAGa5fmM5BzxTk7nOad+w6WYh8Oi5tCz9TmV+mEKYQpZugx2LrtyDIvSEcLT8XaEzcPg9mzEGcXd/jhS8gg8HNqMYlN4x1bVw8unCxQPXC5RKMyXco42f2pszA="

func TestOpenRunMetaFromRecordedVector(t *testing.T) {
	sealed, err := base64.StdEncoding.DecodeString(runMetaVector)
	if err != nil {
		t.Fatalf("样本本身解不开：%v", err)
	}
	got, err := openRunMeta(contract.RunMeta(sealed))
	if err != nil {
		t.Fatalf("开不开：%v", err)
	}
	want := contract.RunMetaBody{
		Argv:              []string{"sshfleet2", "-f", "nodes.csv", "-c", "uptime"},
		Mode:              contract.ModeCommand,
		Command:           "uptime",
		Answers:           []contract.Answer{{Value: "y", Triggers: []string{"continue"}}},
		TimeoutConnect:    10,
		TimeoutExecute:    60,
		ShowCategoryTips:  true,
		ErrorKeywordsPath: "config/keywords_error.conf",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("开出来的内容不对\n got=%+v\nwant=%+v", got, want)
	}
}

func TestOpenRunMetaRejectsShort(t *testing.T) {
	if _, err := openRunMeta(contract.RunMeta{1, 2, 3}); err == nil {
		t.Fatal("长度不够的那一段应当报错，不能当成空内容糊过去")
	}
}

func TestOpenRunMetaRejectsWrongVersion(t *testing.T) {
	sealed, _ := base64.StdEncoding.DecodeString(runMetaVector)
	sealed[0] = 0x7f
	if _, err := openRunMeta(contract.RunMeta(sealed)); err == nil {
		t.Fatal("认不出的版本号应当报错")
	}
}

func TestOpenRunMetaRejectsTampered(t *testing.T) {
	sealed, _ := base64.StdEncoding.DecodeString(runMetaVector)
	sealed[len(sealed)-1] ^= 0xff
	if _, err := openRunMeta(contract.RunMeta(sealed)); err == nil {
		t.Fatal("改过一个字节的那一段应当报错——改密钥漂了也走这条")
	}
}
