# -*- coding: utf-8 -*-
# SSHexec 分类字典文件
# 该文件负责定义分类字典，包括错误分类、警告分类等

""" 错误分类的字典 - 多个关键词到中文分类的映射（注意英文关键词全小写）"""
ERROR_CATEGORIES = {
    "网络不可达": ["no route to host", "network is unreachable", "host is unreachable", "network unreachable", "目标主机不可达", "无法访问目标主机", "无路由到主机"],
    "端口不可达": ["unable to connect to port", "端口不可达"],
    "目标不可达": ["connect call failed", "目标不可达"],
    "密码过期": ["password expired", "change your password now", "has expired", "密码过期"],
    "连接超时": ["timeout", "timed out", "time out", "连接超时", "connect timeout", "connection timeout", "timed out waiting"],
    "连接中断": ["connection lost", "connection closed", "disconnected", "broken pipe", "socket closed", "连接中断", "连接已断开", "管道破裂"],
    "拒绝网络连接": ["connection refused", "connection reset", "refused", "connect refused", "拒绝连接", "连接被拒绝", "拒绝网络连接", "conn refused", "reset by peer"],
    "身份验证失败": ["authentication failed", "password not accepted", "permission denied", "access denied", "认证失败", "密码错误", "身份验证失败", "auth failed"],
    "DNS解析失败": ["name or service not known", "cannot resolve hostname", "dns resolution failed", "unknown host", "hostname not found", "DNS解析错误", "域名解析失败", "无法解析主机名"],
    "SSH协议错误": ["ssh protocol error", "incompatible ssh version", "ssh handshake failed", "protocol mismatch", "SSH协议不兼容", "SSH握手失败", "协议错误"],
    "端口被占用": ["address already in use", "port already in use", "端口被占用", "地址已使用"],
    "防火墙阻止": ["firewall blocked", "connection filtered", "防火墙阻止", "连接被过滤", "blocked by firewall", "filtered"],
    "证书错误": ["certificate error", "SSL error", "TLS handshake failed", "证书错误", "SSL握手失败", "cert verify failed"],
    "协议不支持": ["protocol not supported", "unsupported protocol", "协议不支持"],
    "主机名验证失败": ["hostname verification failed", "hostname mismatch", "主机名验证失败"],
    "连接数限制": ["too many connections", "connection limit exceeded", "连接数过多", "达到连接限制"],
    "网络配置错误": ["network configuration error", "invalid network config", "网络配置错误"]
}

# 高危命令的字典列表 - 关键词到中文分类的映射
DANGEROUS_PATTERNS = [
    # 禁止执行的命令（直接退出）
    {'name': '递归强制删除根目录', 'example': 'rm -rf /', 'regex': r'^\s*rm\s+(-[rf]+|--no-preserve-root|-(?:r\s+f|f\s+r))\s*/\s*(?:#.*)?$', 'risk_level': 'forbidden'},
    {'name': '递归强制删除系统目录', 'example': 'rm -rf /etc', 'regex': r'^\s*rm\s+(-[rf]+|-(?:r\s+f|f\s+r))\s*/(?:etc|usr|var|lib|bin|sbin)(?:/.*)?\s*(?:#.*)?$', 'risk_level': 'forbidden'},
    {'name': '格式化根磁盘', 'example': 'mkfs /dev/sda', 'regex': r'^\s*mkfs\s+/(dev/)?(sd|hd|vd)[a-z]+\s*(?:#.*)?$', 'risk_level': 'forbidden'},
    {'name': '销毁系统数据', 'example': 'dd if=/dev/zero of=/dev/sda', 'regex': r'^\s*dd\s+if=/dev/(?:zero|random)\s+of=/dev/(sd|hd|vd)[a-z]+\s*(?:#.*)?$', 'risk_level': 'forbidden'},
    {'name': '立即关闭系统', 'example': 'shutdown -h now', 'regex': r'^\bshutdown\s+-h\s+now\b', 'risk_level': 'forbidden'},
    {'name': '系统重启', 'example': 'reboot', 'regex': r'\breboot\b', 'risk_level': 'forbidden'},
    {'name': '系统关机', 'example': 'shutdown', 'regex': r'\bshutdown\b', 'risk_level': 'forbidden'},
    {'name': '系统关机2', 'example': 'shutdown -h now', 'regex': r'^\bshutdown\s+-h\s+now\b', 'risk_level': 'forbidden'},
    {'name': '系统关机3', 'example': 'shutdown now', 'regex': r'^\bshutdown\s+now\b', 'risk_level': 'forbidden'},
    {'name': '运行级别0', 'example': 'init 0', 'regex': r'^\b(?:init|telinit)\s+0\b', 'risk_level': 'forbidden'},
    {'name': '运行级别6', 'example': 'init 6', 'regex': r'^\b(?:init|telinit)\s+6\b', 'risk_level': 'forbidden'},
    {'name': '杀死所有进程', 'example': 'kill -9 -1', 'regex': r'^\bkill\s+-9\s+-1\b', 'risk_level': 'forbidden'},
    {'name': '屏蔽关键服务', 'example': 'systemctl mask sshd', 'regex': r'^\bsystemctl\s+mask\s+(?:ssh|network|firewalld)\b', 'risk_level': 'forbidden'},
    {'name': '关闭关键服务', 'example': 'systemctl mask sshd', 'regex': r'^\bsystemctl\s+mask\s+(?:ssh|network)\b', 'risk_level': 'forbidden'},

    # 高风险命令（需要强烈警告）
    {'name': '递归强制删除', 'example': 'rm -rf /tmp/*', 'regex': r'^\s*rm\s+(-(?:r\s+f|f\s+r)|-rf|--no-preserve-root)\b', 'risk_level': 'high'},
    {'name': '强制删除文件', 'example': 'rm -f /var/log/messages', 'regex': r'^\brm\s+-f\s+.*\.(log|conf|cfg)\s*(?:#.*)?$', 'risk_level': 'high'},
    {'name': '格式化文件系统', 'example': 'mkfs.ext4 /dev/sdb1', 'regex': r'^\bmkfs\b', 'risk_level': 'high'},
    {'name': '数据覆盖操作', 'example': 'dd if=file.img of=/dev/sdb1', 'regex': r'^\bdd\s+of=/dev/(sd|hd|vd)[a-z]+\s*(?:#.*)?$', 'risk_level': 'high'},
    {'name': '强制停止服务', 'example': 'systemctl stop nginx', 'regex': r'^\bsystemctl\s+stop\s+(?:mysql|nginx|apache)\b', 'risk_level': 'high'},
    {'name': '清空防火墙规则', 'example': 'iptables -F', 'regex': r'^\biptables\s+-F\b', 'risk_level': 'high'},
    {'name': '删除用户账户', 'example': 'userdel testuser', 'regex': r'^\buserdel\b', 'risk_level': 'high'},
    {'name': '强制软件包删除', 'example': 'rpm -e --nodeps package-name', 'regex': r'^\brpm\s+-e\s+--nodeps\b', 'risk_level': 'high'},

    # 中风险命令（需要警告）
    {'name': '递归删除', 'example': 'rm -r directory', 'regex': r'^\brm\s+-r\b', 'risk_level': 'medium'},
    {'name': '强制删除', 'example': 'rm -f file.txt', 'regex': r'^\brm\s+-f\b', 'risk_level': 'medium'},
    {'name': '权限修改', 'example': 'chmod 777 file.sh', 'regex': r'^\bchmod\s+[0-7][0-7][0-7]7\b', 'risk_level': 'medium'},
    {'name': '设置SUID权限', 'example': 'chmod u+s /usr/bin/script', 'regex': r'^\bchmod\s+u\+s\b', 'risk_level': 'medium'},
    {'name': '设置SGID权限2', 'example': 'chmod g+s /usr/bin/script', 'regex': r'^\bchmod\s+g\+s\b', 'risk_level': 'medium'},
    {'name': '修改文件所有者', 'example': 'chown root:root /etc/passwd', 'regex': r'^\bchown\s+root:root\b', 'risk_level': 'medium'},
    {'name': '禁用系统服务', 'example': 'systemctl disable service-name', 'regex': r'^\bsystemctl\s+disable\b', 'risk_level': 'medium'},
    {'name': '修改内核参数', 'example': 'sysctl -w kernel.parameter=value', 'regex': r'^\bsysctl\s+-w\b', 'risk_level': 'medium'},
    {'name': '锁定用户账户', 'example': 'passwd -l username', 'regex': r'^\bpasswd\s+-l\b', 'risk_level': 'medium'},
    {'name': '删除用户组', 'example': 'groupdel groupname', 'regex': r'^\bgroupdel\b', 'risk_level': 'medium'},
    {'name': '清空系统日志', 'example': 'truncate -s 0 /var/log/messages', 'regex': r'^\btruncate\s+-s\s+0\s+/var/log/', 'risk_level': 'medium'},
    {'name': '删除日志文件', 'example': 'rm -f /var/log/secure', 'regex': r'^\brm\s+-f\s+/var/log/.*\b', 'risk_level': 'medium'},
    {'name': '删除系统文件', 'example': 'rm -f /etc/hosts', 'regex': r'^\brm\s+-f\s+/etc/.*\b', 'risk_level': 'medium'},

    # 低风险命令（需要提示）
    {'name': '软件包更新', 'example': 'yum update', 'regex': r'^\byum\s+update\b', 'risk_level': 'low'},
    {'name': '软件包删除', 'example': 'yum remove package-name', 'regex': r'^\byum\s+remove\b', 'risk_level': 'low'},
    {'name': '强制安装软件包', 'example': 'rpm -i --force package.rpm', 'regex': r'^\brpm\s+-i\s+--force\b', 'risk_level': 'low'},
    {'name': '环境变量设置', 'example': 'export PATH=/new/path:$PATH', 'regex': r'^\bexport\s+[A-Z_]+=', 'risk_level': 'low'},
    {'name': '命令别名设置', 'example': 'alias ll="ls -l"', 'regex': r'^\balias\s+\w+=', 'risk_level': 'low'},
    {'name': '清空命令历史', 'example': 'history -c', 'regex': r'^\bhistory\s+-c\b', 'risk_level': 'low'},
    {'name': '重启Shell', 'example': 'exec $SHELL', 'regex': r'^\bexec\s+\$SHELL\b', 'risk_level': 'low'},
    {'name': '修改文件描述符限制', 'example': 'ulimit -n 65535', 'regex': r'^\bulimit\s+-n\s+65535\b', 'risk_level': 'low'},
    {'name': '修改内存限制', 'example': 'ulimit -v unlimited', 'regex': r'^\bulimit\s+-v\s+unlimited\b', 'risk_level': 'low'},
    {'name': '运行级别切换', 'example': 'init 3', 'regex': r'^\b(?:init|telinit)\s+[1-5]\b', 'risk_level': 'low'},
    {'name': '普通服务重启', 'example': 'systemctl restart nginx', 'regex': r'^\bsystemctl\s+restart\s+\w+\b', 'risk_level': 'low'},
]

