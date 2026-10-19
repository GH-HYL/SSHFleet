package dangercheck

import (
	"testing"
)

// 判定层的可验证边界：每条规则的「必须命中 / 必须不命中」。
// 期望值照工单 04–06 的验收条目人写，不从规则文件反推。

type boundary struct {
	rule   string // 规则名；空串表示「不得被任何规则命中」
	hits   []string
	misses []string
}

func ruleIndex(t *testing.T, rules *Rules, name string) int {
	t.Helper()
	for i, r := range rules.items {
		if r.Name == name {
			return i
		}
	}
	t.Fatalf("规则文件里没有「%s」", name)
	return -1
}

func matchesRule(rules *Rules, idx int, src string) bool {
	text := prepare(normalizeSrc(src))
	return len(rules.hitLines(idx, text, newLineIndex(text))) > 0
}

func TestRuleBoundaries(t *testing.T) {
	rules := loadRulesOrFail(t)
	bs := []boundary{
		{
			rule:   "递归删除根目录",
			hits:   []string{`rm -rf /`, `rm -rf /*`, `rm -rf //`, `rm -rf /.`, `rm -r -f /`, `rm -fr /`, `rm --recursive --force /`, `rm -rf --no-preserve-root /`},
			misses: []string{`rm -rf /etc`, `rm -rf /tmp/x`, `rm -rf /opt/x`, `echo ok`},
		},
		{
			rule:   "递归删除系统目录",
			hits:   []string{`rm -rf /etc`, `rm -rf /etc/`, `rm -rf /etc/*`, `rm -rf /usr`, `rm -rf /boot`},
			misses: []string{`rm -rf /etc/nginx`, `rm -rf /var/log`, `rm -f /etc/hosts`, `rm -rf /opt/x`},
		},
		{
			rule:   "格式化主盘",
			hits:   []string{`mkfs /dev/sda`, `mkfs.ext4 /dev/sda1`, `mkfs.xfs /dev/nvme0n1`},
			misses: []string{`df -h`, `mount /dev/sda1 /mnt`},
		},
		{
			rule:   "抹盘 dd",
			hits:   []string{`dd if=/dev/zero of=/dev/sda`, `dd if=/dev/zero of=/dev/nvme0n1`},
			misses: []string{`dd if=/tmp/a of=/tmp/b`},
		},
		{
			rule:   "关机停机",
			hits:   []string{`shutdown -h now`, `shutdown -P now`, `shutdown now`, `poweroff`, `halt`, `init 0`, `telinit 0`},
			misses: []string{`shutdown -c`, `shutdown +0 -c`, `shutdown -h +10 -c`, `shutdown -r now`},
		},
		{
			rule:   "杀死所有进程",
			hits:   []string{`kill -9 -1`, `kill -KILL -1`},
			misses: []string{`kill -9 1234`},
		},
		{
			rule:   "系统重启",
			hits:   []string{`reboot`, `shutdown -r now`, `init 6`},
			misses: []string{`systemctl restart nginx`},
		},
		{
			rule:   "递归删除",
			hits:   []string{`rm -rf /opt/x`, `rm -fr /opt/x`, `rm -r -f /opt/x`, `rm -R -f /opt/x`, `rm --recursive --force /opt/x`},
			misses: []string{`rm -f /opt/x`}, // TODO(05)：`rm -rf /tmp/x`、`rm -rf /etc` 的排除待定
		},
		{
			rule:   "屏蔽关键服务",
			hits:   []string{`systemctl mask sshd`, `systemctl mask network`},
			misses: []string{`systemctl mask firewalld`, `systemctl stop sshd`},
		},
		{
			rule:   "清空防火墙",
			hits:   []string{`iptables -F`, `ip6tables -F`},
			misses: []string{`iptables -L`},
		},
		{
			rule:   "强制删包",
			hits:   []string{`rpm -e --nodeps pkg`, `yum remove -y pkg`},
			misses: []string{`yum install -y pkg`},
		},
		{
			rule:   "删除关键路径文件",
			hits:   []string{`rm -f /etc/hosts`, `rm -f /var/log/app.log`, `rm -rf /etc/nginx`},
			misses: []string{`rm -f /tmp/x`, `rm -rf /tmp/build_cache`, `rm -f ./a.txt`},
		},
		{
			rule:   "停止·禁用服务",
			hits:   []string{`systemctl stop sshd`, `systemctl disable sshd`, `systemctl stop network`, `systemctl disable firewalld`, `systemctl mask firewalld`},
			misses: []string{`systemctl stop nginx`, `systemctl restart nginx`},
		},
		{
			rule:   "删除账户·用户组",
			hits:   []string{`userdel a`, `userdel -r a`, `groupdel g`},
			misses: []string{`useradd -m a`, `groupadd g`},
		},
		{
			rule:   "权限修改",
			hits:   []string{`chmod 777 f`, `chmod -R 777 /var/www`, `chmod o+w f`},
			misses: []string{`chmod 755 /var/www`, `chmod 775 f`},
		},
		{
			rule:   "SUID·SGID",
			hits:   []string{`chmod u+s /usr/bin/x`, `chmod g+s /usr/bin/x`, `chmod 4755 x`, `chmod 2755 x`},
			misses: []string{`chmod 755 x`},
		},
		{
			rule:   "清空系统日志",
			hits:   []string{`truncate -s 0 /var/log/messages`, `> /var/log/messages`},
			misses: []string{`tail -f /var/log/messages`},
		},
	}
	for _, b := range bs {
		idx := ruleIndex(t, rules, b.rule)
		for _, s := range b.hits {
			if !matchesRule(rules, idx, s) {
				t.Errorf("「%s」应命中：%q", b.rule, s)
			}
		}
		for _, s := range b.misses {
			if matchesRule(rules, idx, s) {
				t.Errorf("「%s」不应命中：%q", b.rule, s)
			}
		}
	}
}
