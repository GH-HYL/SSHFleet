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
_sshfleet() {
    local prev=${COMP_WORDS[COMP_CWORD-1]}
    local cur=${COMP_WORDS[COMP_CWORD]}

    case "$prev" in
        -f|--csv-file|-s|--script|-u|--upload|-a|--answer|--change-password|--convert-secret)
            return 1 ;;
        -c|--command|-d|--download|-r|--remark|-t|--timeout|-T|--connect-timeout|-n|--number)
            COMPREPLY=(); return 0 ;;
        -p|--path)
            local IFS=$'\n'
            COMPREPLY=($(compgen -d -- "$cur")); return 0 ;;
    esac

    COMPREPLY=($(compgen -W "@OPTIONS@" -- "$cur"))
}
complete -o bashdefault -o default -F _sshfleet SSHFleet
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
