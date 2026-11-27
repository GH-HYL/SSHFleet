#!/bin/sh
# 装 SSHFleet：把程序放到能直接敲 SSHFleet 的位置，并装上命令补全。
# 解压后在当前目录执行 ./install.sh，不需要加参数。

set -u

APP=SSHFleet
DIR=$(cd "$(dirname "$0")" && pwd)
SRC=$DIR/$APP

say() { printf '%s\n' "$1"; }
die() { printf '%s\n' "$1" >&2; exit 1; }

# 往启动文件里加一行，带标记，加过一次就不再重复
add_line() {
    [ -f "$1" ] || touch "$1" 2>/dev/null || return 0
    grep -qF -- "$2" "$1" 2>/dev/null && return 0
    printf '\n%s\n%s\n' "$2" "$3" >> "$1"
}

[ -f "$SRC" ] || die "安装包里没有 SSHFleet 文件，把它和 install.sh 放在同一目录再执行。"

if [ "$(id -u)" -eq 0 ]; then
    BIN_DIR=/bin
    COMP_DIR=/usr/share/bash-completion/completions
    RC=
else
    BIN_DIR=$HOME/.local/bin
    COMP_DIR=$HOME/.local/share/bash-completion/completions
    RC=$HOME/.bashrc
fi

mkdir -p "$BIN_DIR" 2>/dev/null ||
    die "没有写 $BIN_DIR 的权限。改用普通用户执行，会装到你自己的目录。"
mkdir -p "$COMP_DIR" 2>/dev/null ||
    die "没有写 $COMP_DIR 的权限。改用普通用户执行，会装到你自己的目录。"

# 原有的同名文件由 mv 问一句。答 n 时 mv 不改文件、退出码仍是 0，所以按源文件还在不在判
if [ -e "$BIN_DIR/$APP" ]; then
    mv -i -- "$SRC" "$BIN_DIR/$APP" || die "没能写入 $BIN_DIR/$APP。确认磁盘空间和权限后重试。"
    if [ -e "$SRC" ]; then
        die "没有覆盖 $BIN_DIR/$APP，安装停止。要替换旧版本，重新执行，在提示处输入 y。"
    fi
    replaced=1
else
    mv -- "$SRC" "$BIN_DIR/$APP" || die "没能写入 $BIN_DIR/$APP。确认磁盘空间和权限后重试。"
    replaced=0
fi
chmod 755 "$BIN_DIR/$APP"
# 从解压目录搬过来的文件还属于原用户，装到系统目录后改成 root 所有，别的账号才改不动它
if [ "$(id -u)" -eq 0 ]; then
    chown root:root "$BIN_DIR/$APP" 2>/dev/null || true
fi

cat > "$COMP_DIR/$APP" <<'EOF'
# SSHFleet 的命令补全，由安装脚本生成。
# 互斥关系与 internal/gate/run.go 的 argumentRefusal 对应：那边改了，这里一起改。

declare -A _sshfleet_long=(@ALIAS_MAP@)

# 已给的一项与候选是否互斥；同一个选项的两种写法算重复，也返回真
_sshfleet_clash() {
    local g=${_sshfleet_long[$1]:-$1} c=${_sshfleet_long[$2]:-$2}
    [ "$g" = "$c" ] && return 0

    # 五种执行模式互斥
    case " @MODES@ " in
        *" $g "*)
            case " @MODES@ " in *" $c "*) return 0 ;; esac ;;
    esac

    # 三条密钥命令之间互斥，且与批量执行参数互斥
    case " @KEYTOOLS@ " in
        *" $g "*)
            case " @KEYTOOLS@ @BATCH@ " in *" $c "*) return 0 ;; esac ;;
    esac
    case " @KEYTOOLS@ " in
        *" $c "*)
            case " @BATCH@ " in *" $g "*) return 0 ;; esac ;;
    esac

    case "$g" in
        --change-password)
            case "$c" in --answer|--path|--key|--no-shell|--sudo|--no-sudo) return 0 ;; esac ;;
        --no-shell)
            case "$c" in --script|--upload|--download|--change-password) return 0 ;; esac ;;
        --script)
            case "$c" in --path|--no-shell|--answer) return 0 ;; esac ;;
        --upload|--download)
            case "$c" in --no-shell|--answer) return 0 ;; esac ;;
        --answer)
            case "$c" in --upload|--download|--no-shell|--change-password) return 0 ;; esac ;;
        --command)
            case "$c" in --path) return 0 ;; esac ;;
        --path)
            case "$c" in --command|--script|--change-password) return 0 ;; esac ;;
        --key)
            [ "$c" = --change-password ] && return 0 ;;
        --sudo)
            case "$c" in --no-sudo|--change-password) return 0 ;; esac ;;
        --no-sudo)
            case "$c" in --sudo|--change-password) return 0 ;; esac ;;
    esac
    return 1
}

_sshfleet() {
    local prev=${COMP_WORDS[COMP_CWORD-1]}
    local cur=${COMP_WORDS[COMP_CWORD]}

    # 第一个参数位置还什么都没敲：不出候选
    if [ "$COMP_CWORD" -eq 1 ] && [ -z "$cur" ]; then
        COMPREPLY=()
        return 0
    fi

    case "$prev" in
        # 要本机文件的位置：自己列文件，不靠回落
        -s|--script|-u|--upload|-f|--csv-file|--convert-secret)
            compopt -o filenames
            mapfile -t COMPREPLY < <(compgen -f -- "$cur")
            return 0 ;;
        # 自由填写的位置（命令、远端路径、秒数、节点号、备注、密码、代填内容）：一条都不出
        -c|--command|-d|--download|-p|--path|-r|--remark|-t|--timeout|-T|--connect-timeout|-n|--number|--change-password|-a|--answer)
            COMPREPLY=()
            return 0 ;;
    esac

    # 选项位：剔掉已给过的和跟它互斥的
    local given="" o w out="" bad
    for w in "${COMP_WORDS[@]:1:COMP_CWORD-1}"; do
        case "$w" in -*) given="$given $w" ;; esac
    done
    for o in @OPTIONS@; do
        bad=0
        for w in $given; do
            _sshfleet_clash "$w" "$o" && { bad=1; break; }
        done
        [ "$bad" -eq 0 ] && out="$out $o"
    done
    COMPREPLY=($(compgen -W "$out" -- "$cur"))
}
complete -F _sshfleet SSHFleet
EOF

path_added=0
if [ -n "$RC" ]; then
    case ":$PATH:" in
        *":$BIN_DIR:"*) ;;
        *)
            add_line "$RC" '# SSHFleet：把 ~/.local/bin 放进 PATH' \
                'export PATH="$HOME/.local/bin:$PATH"'
            path_added=1
            ;;
    esac
fi

# 系统没有 bash-completion 的自动加载时，补一行 source，否则补全文件不会被读到
if [ ! -f /usr/share/bash-completion/bash_completion ]; then
    if [ -n "$RC" ]; then
        add_line "$RC" '# SSHFleet：加载命令补全' \
            '. "$HOME/.local/share/bash-completion/completions/SSHFleet"'
    else
        [ -f /etc/profile.d/sshfleet-completion.sh ] ||
            printf '. %s\n' "$COMP_DIR/$APP" > /etc/profile.d/sshfleet-completion.sh
    fi
fi

say "SSHFleet 已安装：$BIN_DIR/$APP"
if [ "$replaced" -eq 1 ]; then
    say "已替换旧版本：$BIN_DIR/$APP"
fi

case "${SHELL:-}" in
    */bash)
        say "命令补全已安装：新开一个终端，输入 SSHFleet - 再按 Tab"
        ;;
    *)
        shell_name=${SHELL##*/}
        say "补全只对 bash 有效，你现在用的是 ${shell_name:-未知 shell}；SSHFleet 已装好，补全需要另配。"
        ;;
esac

if [ "$path_added" -eq 1 ]; then
    say "~/.local/bin 不在 PATH 里，已加进 ~/.bashrc，重新登录后生效。"
    say '本次就想用，执行：export PATH="$HOME/.local/bin:$PATH"'
fi

exit 0
