# -*- coding: utf-8 -*-
# SSHexec 传输文件
# 该文件负责处理文件传输操作，包括上传和下载

# 系统或第三方模块
import os
import signal
from collections import Counter
from datetime import datetime
from typing import List, Dict, Any, Tuple, Optional

from fabric import Connection
from rich.console import Console
from rich.live import Live
from rich.progress import Progress, BarColumn, TextColumn, TimeRemainingColumn, TimeElapsedColumn, TransferSpeedColumn, DownloadColumn, TaskID
from rich.table import Table

# 自定义模块
import src.transfer.transfer_check as transfer_check
import src.transfer.transfer_utils as transfer_utils
import src.utils as utils
from src.utils import elog

# 全局console对象，进度条
console=Console()


def sftp_upload(conn: Connection, node_info: Dict[str, Any], local_path: str, remote_path: str, transfer_progress: Progress, task_id: TaskID, info_progress: Progress, info_task: TaskID, callable_tar_result: Tuple[int, int, Optional[str]], use_sudo: bool) -> None:
    """
    功能：
        使用SFTP上传文件的核心函数

    参数：
        conn: 已建立的SSH连接
        node_info: 节点信息字典，包含ip/port/user/password等
        local_path: 本地路径
        remote_path: 远程路径(必须是绝对路径的目录)
        transfer_progress: 进度条对象
        task_id: 进度条任务ID
        info_progress: 进度条对象
        info_task: 进度条任务ID
        use_sudo: 是否使用sudo

    返回：
        None
    """

    # 获取node_info['user'] 的家目录 的绝对地址
    try:
        home_dir = conn.run(f"echo ~{node_info['user']}", hide=True).stdout.strip()
        elog.info(f'获取家目录成功：{home_dir}')
    except Exception as e:
        elog.error(f'获取家目录失败：{e}')
        raise Exception("获取家目录失败")

    # chmod_chown_dir = f'{home_dir}/.sshexec_packages'
    large_file_threshold = 100 * 1024 * 1024  # 100MB

    def progress_callback(bytes_sent, _):
        """回调函数：基于累计字节数更新进度"""
        transfer_progress.update(task_id, completed=bytes_sent)


    try:
        info_progress.update(info_task, status='传输中')
        with conn.sftp() as sftp:
            elog.info('打开sftp成功')
            
            # 改进的重名检测逻辑
            local_files = set()
            
            # 检查远程路径是否存在
            test_command = f"test -e '{remote_path}'"
            if use_sudo:
                test_result = conn.sudo(test_command, hide=True, warn=True)
            else:
                test_result = conn.run(test_command, hide=True, warn=True)
                
            if test_result.ok:
                elog.info('remote_path远程路径存在')
                
                # 获取远程文件列表（改进版本）
                try:
                    if use_sudo:
                        # 使用更可靠的命令获取相对路径
                        ls_result = conn.sudo(f"find {remote_path} -type f -printf '%P\\n'", hide=True, warn=True)
                    else:
                        ls_result = conn.run(f"find {remote_path} -type f -printf '%P\\n'", hide=True, warn=True)
                    
                    # 过滤空字符串并规范化路径
                    remote_files = {path.strip() for path in ls_result.stdout.split('\n') if path.strip()}
                    elog.info(f'获取remote_path远程路径下所有文件成功，共{len(remote_files)}个文件')

                except Exception as e:
                    elog.error(f'获取remote_path远程路径下所有文件失败：{e}')
                    raise Exception("获取远程文件名失败")

                # 获取本地文件列表（改进版本）
                if os.path.isfile(local_path):
                    # 单个文件，直接使用相对于local_path的文件名（空字符串）
                    local_files.add(os.path.basename(local_path))
                else:
                    for root, dirs, files in os.walk(local_path):
                        # 检查符号链接
                        for name in list(dirs):
                            full_path = os.path.join(root, name)
                            if os.path.islink(full_path):
                                dirs.remove(name)
                        for file in files:
                            file_path = os.path.join(root, file)
                            if not os.path.islink(file_path):
                                # 计算相对于local_path的相对路径，并规范化
                                rel_path = os.path.relpath(file_path, local_path)
                                # 统一使用Unix风格路径分隔符
                                rel_path = rel_path.replace('\\', '/')
                                # 确保路径不以./开头
                                if rel_path.startswith('./'):
                                    rel_path = rel_path[2:]
                                local_files.add(rel_path)

                duplicate_items = local_files & remote_files

                if duplicate_items:
                    elog.info(f'发现 {len(duplicate_items)} 个重名文件')
                    elog.info(f'重名文件示例: {list(duplicate_items)[:5]}')  # 只显示前5个
                    raise Exception("存在重名文件")
                else:
                    elog.info('无重名文件，可安全上传')
            else:
                elog.info('remote_path远程路径不存在，可安全上传')

            # 创建临时目录（确保父目录存在）
            # 拼接临时目录绝对路径
            tmp_dir = f"{home_dir}/.sshexec_packages/upload_tmp/{os.urandom(4).hex()}"
            
            if transfer_utils.create_dir(conn, tmp_dir, use_sudo=False):
                elog.info(f'创建临时目录成功：{tmp_dir}')
            else:
                elog.error(f'创建临时目录失败：{tmp_dir}')
                raise Exception("创建临时目录失败")

            # 文件/目录上传到临时目录
            if os.path.isfile(local_path):
                elog.info(f'上传内容为单个文件：{local_path}')
                remote_file = f"{tmp_dir}/{os.path.basename(local_path)}"
                # 文件传输
                if callable_tar_result[0] > large_file_threshold:
                    elog.info(f'上传文件为大文件，超过设定的值：{large_file_threshold}，采用流式传输')
                    with open(local_path, 'rb') as f:
                        sftp.putfo(f, remote_file, callback=progress_callback)
                else:
                    sftp.put(local_path, remote_file, callback=progress_callback)
                elog.info(f'上传文件成功 {remote_file}')
            else:
                elog.info(f'上传内容为目录：{local_path}')
                try:
                    if callable_tar_result[2] is None:
                        elog.error(f'上传tar包失败，上传任务中，本地为目录，但未正确生成压缩文件路径{callable_tar_result[2]}')
                        raise Exception("压缩文件未正常生成")
                    # 上传tar包
                    remote_tar_path = f"{tmp_dir}/{os.path.basename(callable_tar_result[2])}"
                    if callable_tar_result[1] > large_file_threshold:
                        elog.info(f'上传tar包为大文件，超过设定的值：{large_file_threshold}，采用流式传输')
                        # 使用本地tar文件路径
                        with open(callable_tar_result[2], 'rb') as f:
                            # remote_tar_path 是远程路径
                            sftp.putfo(f, remote_tar_path, callback=progress_callback)
                    else:
                        sftp.put(callable_tar_result[2], remote_tar_path, callback=progress_callback)
                    elog.info(f'上传tar包成功 {remote_tar_path}')
                except Exception as e:
                    elog.error(f'上传tar包失败\n异常类型：\n{type(e)}\n异常信息：\n{e}')
                    raise Exception(f"上传tar包失败: {e}")
                


        # 创建 remote_path 确保路径存在
        transfer_utils.create_dir(conn, remote_path, use_sudo)
        # 移动/解压文件到最终位置
        try:
            if os.path.isfile(local_path):
                # 单个文件：从临时目录移动到目标目录
                filename = os.path.basename(local_path)
                source_file = f"{tmp_dir}/{filename}"  # 文件在临时目录
                mv_cmd = f"mv -n '{source_file}' '{remote_path}' 2>&1"
                elog.info(f"移动文件命令: {mv_cmd}")
            else:
                # 目录：从临时目录解压tar包到目标目录
                if not callable_tar_result[2]:
                    raise Exception("tar文件路径为空，无法解压")
                
                tar_filename = os.path.basename(callable_tar_result[2])
                source_tar = f"{tmp_dir}/{tar_filename}"  # tar包在临时目录
                mv_cmd = f"tar -xzf '{source_tar}' -C '{remote_path}' --strip-components=1 2>&1"
                elog.info(f"解压tar包命令: {mv_cmd}")

            if use_sudo:
                mv_result = conn.sudo(mv_cmd, hide=True, warn=True)
            else:
                mv_result = conn.run(mv_cmd, hide=True, warn=True)
                
            if mv_result.exited == 0:
                elog.info('移动/解压文件成功')
                
                # 移动完，统一修改临时目录的权限和所有者
                file_user= "root" if use_sudo else node_info["user"]
                transfer_utils.chmod_chown_remote_files(conn, remote_path, file_user, use_sudo)

            else:
                elog.error(f'移动/解压文件失败，输出内容为：{mv_result.stdout}')
                elog.error(f'错误信息：{mv_result.stderr}')
                raise Exception(f"移动/解压文件失败: {mv_result.stderr}")
                
        except Exception as e:
            elog.error(f'移动/解压文件失败，异常信息：{e}')
            raise Exception(f"移动/解压文件失败: {e}")

        finally:
            # 清理目标临时目录（保持不变）
            try:
                clear_dir = f"{home_dir}/.sshexec_packages"
                cleanup_cmd = f"rm -rf {clear_dir}"
                if use_sudo:
                    conn.sudo(cleanup_cmd, hide=True, warn=True)
                else:
                    conn.run(cleanup_cmd, hide=True, warn=True)
                elog.info(f'删除目标临时目录成功 {clear_dir}')
            except Exception as e:
                elog.error(f'删除目标临时目录失败 {clear_dir}，异常信息：{e}')

    finally:

        # 清理目标临时目录
        try:
            clear_dir = f"{home_dir}/.sshexec_packages"
            cleanup_cmd = f"rm -rf {clear_dir}"
            if use_sudo:
                conn.sudo(cleanup_cmd, hide=True, warn=True)
            else:
                conn.run(cleanup_cmd, hide=True, warn=True)
            elog.info(f'删除目标临时目录成功 {clear_dir}')
            
        except Exception as e:
            elog.error(f'删除目标临时目录失败 {clear_dir}\n异常类型：\n{type(e)}\n异常信息：\n{e}')


# noinspection PyUnusedLocal
def sftp_download(
    conn: Connection,
    node_info: Dict[str, Any],
    remote_path: str,
    local_path: str,
    transfer_progress: Progress,
    task_id: TaskID,
    use_sudo: bool = False
) -> None:
    """
    使用SFTP下载文件的核心函数
    Args:
        conn: 已建立的SSH连接
        node_info: 节点信息字典
        remote_path: 远程路径
        local_path: 本地存储路径
        transfer_progress: 进度条对象
        task_id: 进度条任务ID
        use_sudo: 是否使用sudo
    """
    # 初始化变量
    # base_name = os.path.basename(remote_path.rstrip('/'))
    # renamed_local_path = f"{base_name}_{conn.host}"
    # final_local_path = posix_join(local_path, renamed_local_path)
    
    try:
        with conn.sftp():
            elog.info('打开sftp成功')
            
            # 检查远程路径是否存在
            test_command = f"test -e '{remote_path}'"
            if use_sudo:
                test_result = conn.sudo(test_command, hide=True, warn=True)
            else:
                test_result = conn.run(test_command, hide=True, warn=True)
                
            if not test_result.ok:
                elog.error(f'远程路径不存在：{remote_path}')
                raise Exception("远程路径不存在")
                
            elog.info(f'远程路径存在：{remote_path}')
            
            # 创建本地目录（如果不存在）
            os.makedirs(local_path, exist_ok=True)
            elog.info(f'创建本地目录成功：{local_path}')

            # def process_item(remote_item: str, local_item: str) -> None:
            #     """递归处理远程项目(文件/目录)"""
            #     nonlocal interrupted
            #     if interrupted:
            #         raise Exception("传输被中断")
                    
            #     attrs = sftp.stat(remote_item)
                
            #     # 处理符号链接
            #     if attrs.st_mode & 0o170000 == 0o120000:
            #         link_target = sftp.readlink(remote_item)
            #         if not link_target.startswith(('..', '/')):
            #             os.symlink(link_target, local_item)
            #         transfer_progress.update(task_id, advance=1)
            #         elog.debug(f'处理符号链接成功：{remote_item} -> {local_item}')
            #         return
                
            #     # 处理目录
            #     if attrs.st_mode & 0o40000:
            #         os.makedirs(local_item, exist_ok=True)
            #         elog.debug(f'创建本地目录：{local_item}')
            #         for item in sftp.listdir(remote_item):
            #             process_item(
            #                 posix_join(remote_item, item),
            #                 posix_join(local_item, item)
            #             )
                
            #     # 处理文件
            #     else:
            #         with sftp.file(remote_item, 'rb') as remote_f:
            #             with open(local_item, 'wb') as local_f:
            #                 while True:
            #                     if interrupted:
            #                         raise Exception("传输被中断")
            #                     data = remote_f.read(32768)
            #                     if not data:
            #                         break
            #                     local_f.write(data)
            #                     transfer_progress.update(task_id, advance=len(data))
            #         elog.info(f'下载文件成功：{remote_item} -> {local_item}')
            
            # process_item(remote_path, final_local_path)
            # elog.info(f'下载完成：{remote_path} -> {final_local_path}')

    except Exception as e:
        elog.error(f'下载过程中出现异常：{e}')
        raise Exception(f"下载失败: {e}")


def upload_file_or_dir(node_info: Dict[str, Any], args: Any, result: Dict[str, Any], transfer_progress: Progress, task_id: TaskID, 
                        info_progress: Progress, info_task: TaskID, callable_tar_result: Tuple[int, int, Optional[str]], use_sudo: bool) -> Dict[str, Any]:
    """
    功能：
        上传文件或目录到远程节点

    参数：
        node_info: 节点信息字典
        args: 命令行参数
        result: 结果字典
        transfer_progress: 进度条对象
        task_id: 进度条任务ID
        info_progress: 进度条对象
        info_task: 进度条任务ID
        callable_tar_result: 传输文件的原始大小, 压缩后大小, 临时压缩文件路径
        use_sudo: 是否使用sudo

    返回：
        包含传输结果的字典
    """

    result["thread_start_time"] = datetime.now()
    
    try:
        # 创建连接对象并使用with语句管理
        with transfer_utils.create_ssh_connection(node_info, args) as conn:
            try:
                conn_result = conn.run(":", hide=True)
                elog.info("建立连接成功")
                elog.info(f"检查连接返回信息：{conn_result.stdout.strip()}")
            except Exception as e:
                elog.error(f"建立连接失败\n异常类型：\n{type(e)}\n异常信息：\n{e}")
                result["connect_bool"] = False
                result["exit_bool"] = False
                result["result_category"] = utils.error_classify(node_info.get('ip', '未知IP'), str(e))
                return result
            
            info_progress.update(info_task, status='检查中')
            
            # 预检查执行
            try:
                transfer_check.perform_pre_checks(conn, args.p, callable_tar_result[0], use_sudo, result)
                elog.info("预检查执行通过")
            except Exception as e:
                elog.error(f"预检查执行异常\n异常类型：\n{type(e)}\n异常信息：\n{e}")
                result["exit_bool"] = False
                result["result_category"] = str(e)
                return result
            
            # 开始上传
            try:
                sftp_upload(conn, node_info, args.u, args.p, transfer_progress, task_id, info_progress, info_task, callable_tar_result, use_sudo)
                elog.info("单个节点的上传文件或目录任务成功完成")
                result["exit_bool"] = True
                result["result_category"] = "传输成功"
            except Exception as e:
                elog.error(f"上传文件或目录失败\n异常类型：\n{type(e)}\n异常信息：\n{e}")
                result["exit_bool"] = False
                result["result_category"] = str(e)
                return result
            
    except Exception as e:
        elog.error(f"上传文件或目录失败，出现未识别的异常信息：\n异常类型：\n{type(e)}\n异常信息：\n{e}")
        result["result_category"] = "出现未识别异常" if not result["result_category"] else result["result_category"]
            
    finally:
        # 不再需要手动清理资源，with语句已处理
        result["thread_end_time"] = datetime.now()
        result["thread_cost_time"] = round((result["thread_end_time"] - result["thread_start_time"]).total_seconds(), 3)
    
    return result


# noinspection PyUnusedLocal
def download_file_or_dir(node_info: Dict[str, Any], args: Any, result: Dict[str, Any], transfer_progress: Progress, task_id: TaskID, use_sudo: bool) -> Dict[str, Any]:
    """
    从远程节点下载文件或目录
    Args:
        node_info: 节点信息字典
        args: 命令行参数
        result: 结果字典
        transfer_progress: 进度条对象
        task_id: 进度条任务ID
        use_sudo: 是否使用sudo
    Returns:
        包含传输结果的字典
    """
    # 结果字典初始化
    result = transfer_utils.initialize_result_dict(node_info, args, download=True)
    result["thread_start_time"] = datetime.now()
    
    try:
        # 创建连接对象
        try:
            conn = transfer_utils.create_ssh_connection(node_info, args)
            elog.debug("创建连接对象成功")
        except Exception as e:
            elog.error(f"创建连接对象失败，异常信息：{e}")
            utils.print_error_informantion_and_exit('create_ssh_connection',f'创建连接对象失败：{e}')
        
        # 建立连接
        try:
            conn.open()
            result["connect_bool"] = True
            elog.info("建立连接成功")
        except Exception as e:
            elog.error(f"建立连接失败，异常信息：{e}")
            result["connect_bool"] = False
            result["exit_bool"] = False
            result["result_category"] = utils.error_classify(node_info.get('ip', '未知IP'), str(e))
            return result

        # 检查远程操作系统
        # try:
        #     transfer_check.check_unix_os(conn)
        #     elog.debug("检查远程操作系统成功")
        # except Exception as e:
        #     elog.error(f"检查远程操作系统失败，非Unix系统，检查/bin和/usr目录是否存在，异常信息：{e}")
        #     result["exit_bool"] = False
        #     result["result_category"] = "非Unix系统"
        #     return result
        
        # 检查下载路径是否存在
        try:
            transfer_check.check_path_exists(conn, args.d)
            elog.info("下载路径存在")
        except Exception as e:
            elog.error(f"下载路径不存在，异常信息：{e}")
            result["exit_bool"] = False
            result["result_category"] = "下载路径不存在"
            return result

        # 估算远程文件大小
        try:
            size_cmd = f"sudo du -sb {args.d} | cut -f1" if use_sudo else f"du -sb {args.d} | cut -f1"
            size_result = conn.run(size_cmd, hide=True)
            total_size = int(size_result.stdout.strip())
            elog.info(f'估算远程文件大小成功：{total_size}字节')
        except Exception as e:
            elog.error(f'估算远程文件大小失败，异常信息：{e}')
            result["exit_bool"] = False
            result["result_category"] = "估算文件大小失败"
            return result
        
        # 更新进度条
        transfer_progress.update(task_id, description=f"下载 {node_info['ip']}", total=total_size, ip=node_info["ip"])
        
        # 执行下载
        try:
            sftp_download(conn, node_info, args.d, args.p, transfer_progress, task_id, use_sudo)
            elog.info("单个节点的下载文件或目录任务成功完成")
            result["exit_bool"] = True
            result["result_category"] = "传输成功"
            
        except Exception as e:
            elog.error(f"下载文件或目录失败，异常信息：{e}")
            result["exit_bool"] = False
            result["result_category"] = str(e)
            return result
        
    finally:
        # transfer_utils.cleanup_resources(conn)
        result["thread_end_time"] = datetime.now()
        result["thread_cost_time"] = round((result["thread_end_time"] - result["thread_start_time"]).total_seconds(), 3)
    
    return result



def execute_transfer(args: Any, nodesinfos: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    """
    功能：
        执行文件或目录的传输任务
    参数：
        args: 命令行参数
        nodesinfos: 节点信息列表

    返回：
        包含传输结果的字典列表
    """

    # 注册全局中断信号处理器
    signal.signal(signal.SIGINT, transfer_utils.handle_interrupt)  # pyright: ignore[reportArgumentType]
    signal.signal(signal.SIGTERM, transfer_utils.handle_interrupt)  # pyright: ignore[reportArgumentType]

    elog.info(f"开始执行transfer，参数：{args}")

    # 初始化参数
    use_sudo = True if args.m == "sudo" else False

    # 上传文件的原始大小、压缩大小、压缩文件路径计算
    callable_tar_result = transfer_utils.calculate_tar_files(args.u) if hasattr(args, 'u') else (0, 0, None)
    if callable_tar_result[2] is None and os.path.isdir(args.u):
        elog.error(f'上传tar包失败，本地为目录{args.u}，但未正确生成压缩文件路径{callable_tar_result[2]}')
        utils.print_error_informantion_and_exit('calculate_files_size',f'上传tar包失败，上传任务中，本地为目录{args.u}，但未正确生成压缩文件路径{callable_tar_result[2]}')

    # 创建进度条progress
    info_progress = Progress(
        TextColumn("       "),
        TextColumn("{task.fields[ip]}", justify="right"),
        TextColumn("{task.fields[status]}", justify="right"),
        TextColumn("  {task.fields[statistics]}", justify="right"),
    )
    transfer_progress = Progress(
        TextColumn("       "),
        BarColumn(bar_width=40, complete_style="green", finished_style="blue"),
        "[progress.percentage]{task.percentage:>3.0f}%",
        DownloadColumn(),  # 自动处理文件大小单位转换
        TimeRemainingColumn(),
        TransferSpeedColumn(),
    )
    
    node_progress = Progress(
        TextColumn("       "),
        BarColumn(bar_width=40, complete_style="green", finished_style="blue"),
        "[progress.percentage]{task.percentage:>3.0f}%",
        "[green]{task.completed}/{task.total}",
        TimeElapsedColumn(),
    )

    # 创建表格布局
    progress_table = Table.grid()
    progress_table.add_row(info_progress)
    progress_table.add_row(transfer_progress)
    progress_table.add_row(node_progress)

    # 添加任务
    info_task = info_progress.add_task("", ip="", status="", statistics="")
    transfer_task = transfer_progress.add_task("", total=0)
    node_task = node_progress.add_task("", total=len(nodesinfos))
    
    results = []
    
    try:
        # 使用 Rich 库的语法，用于创建一个实时更新的控制台界面
        with Live(progress_table, console=console, refresh_per_second=10):
            elog.info("开始执行进度条progress任务")
            for i, node_info in enumerate(nodesinfos):
                
                # 结果字典初始化
                result = transfer_utils.initialize_result_dict(node_info, args, upload=True)
                
                elog.info(f"开始处理节点：ip {node_info['ip']} 端口 {node_info['port']} 账号 {node_info['user']}，当前执行顺序i值为：{i}")
                
                # 进度条的成功/失败统计
                exit_counts = Counter(d.get('exit_bool') for d in results)
                success_counts = exit_counts.get(True, 0)
                fail_counts = exit_counts.get(False, 0)    
                statistics_str = f'[bright_yellow]Success: [bright_black]{success_counts}[bright_yellow] Fail: [bright_black]{fail_counts}'

                # 更新进度条描述
                info_progress.update(info_task, ip=f'[bright_yellow]Current: [bright_black]{node_info["ip"]}', status='准备中', statistics=statistics_str)
                node_progress.update(node_task, completed=i, total=len(nodesinfos))
                elog.debug("完成初始更新进度条progress任务")
                    
                if transfer_utils.transfer_interrupted:
                    elog.error(f"中断处理，节点{node_info['ip']}传输任务被中断")
                    result.update({
                        "connect_bool": False if result["connect_bool"] is None else result["connect_bool"],
                        "exit_bool": False if result["exit_bool"] is None else result["exit_bool"],
                        "result_category": "传输被Ctrl+C中断" if result["result_category"] is None else result["result_category"]
                    })
                    results.append(result)
                    node_progress.update(node_task, completed=len(nodesinfos))
                    continue

                try:
                    # 执行传输
                    if hasattr(args, 'u') and args.u:
                        transfer_progress.update(transfer_task, completed=0, total=callable_tar_result[1])
                        elog.info("开始上传文件或目录")
                        result = upload_file_or_dir(node_info, args, result, transfer_progress, transfer_task, info_progress, info_task, callable_tar_result, use_sudo)
                    elif hasattr(args, 'd') and args.d:
                        elog.info("开始下载文件或目录")
                        result = download_file_or_dir(node_info, args, result, transfer_progress, transfer_task, use_sudo)
                    else:
                        elog.error(f"参数错误,请联系开发者检查args.u和.args.d值的有效性，当前值：{args.u},{args.d}")
                        utils.print_error_informantion_and_exit("execute_transfer", f"参数错误,请联系开发者检查args.u和.args.d值的有效性，当前值：{args.u},{args.d}")

                    # 单节点完成传输后，更新进度条progress任务
                    node_progress.update(node_task, completed=i+1)
                    results.append(result)
                    elog.info(f"完成{node_info['ip']}传输任务，返回结果字典到字典列表，更新进度条progress任务")


                except KeyboardInterrupt:
                    elog.error("检测到Ctrl+C中断信号，except正在处理后续资源回收")
                    print("\033[91m检测到Ctrl+C中断信号，except正在处理后续资源回收\033[0m")
                    result.update({
                        "connect_bool": False if result["connect_bool"] is None else result["connect_bool"],
                        "exit_bool": False if result["exit_bool"] is None else result["exit_bool"],
                        "result_category": "传输被Ctrl+C中断" if result["result_category"] is None else f'{result["result_category"]}，传输被用户Ctrl+C中断'
                    })

                    results.append(result)
                    elog.error("中断处理，更新结果字典列表完成")

                    transfer_progress.remove_task(transfer_task)
                    elog.error("中断处理，移除传输进度条任务")
                    node_progress.remove_task(node_task)
                    elog.error("中断处理，移除节点进度条任务")


                except Exception as e:
                    elog.error(f"传输过程中出现异常，异常信息：{e}")
                    elog.error("传输过程中出现异常，更新结果字典")
                    result.update({
                        "connect_bool": False if result["connect_bool"] is None else result["connect_bool"],
                        "exit_bool": False if result["exit_bool"] is None else result["exit_bool"],
                        "result_category": f"传输失败，执行过程中出现异常： {e}" if result["result_category"] is None else result["result_category"]
                    })
                    results.append(result)
                    elog.error("传输过程中出现异常，更新结果字典列表完成")

                    continue

            # 所有节点传输完成后，更新进度条progress任务
            exit_counts = Counter(d.get('exit_bool') for d in results)
            success_counts = exit_counts.get(True, 0)
            fail_counts = exit_counts.get(False, 0)    
            statistics_str = f'[bright_yellow]Success: [bright_black]{success_counts}[bright_yellow] Fail: [bright_black]{fail_counts}'
            info_progress.update(info_task, ip=f'[bright_yellow]Current: [bright_black]{node_info["ip"]}', status='已结束', statistics=statistics_str)
            elog.info("所有节点传输完成，更新进度条progress任务")

    except Exception as e:
        error_msg = f"{type(e).__name__}: {e}"
        elog.error(f"实时更新的控制台界面后出现异常，异常信息：{error_msg}")
        result.update({
            "connect_bool": False if result["connect_bool"] is None else result["connect_bool"],
            "exit_bool": False if result["exit_bool"] is None else result["exit_bool"],
            "result_category": f"传输失败，异常信息未识别分类：{type(e).__name__}"
        })
        results.append(result)
        elog.error("实时更新的控制台界面后出现异常，更新结果字典列表完成")
        
    finally:
        # 清理本地临时压缩包
        if callable_tar_result[2] and os.path.exists(callable_tar_result[2]):
            try:
                os.remove(callable_tar_result[2])
                elog.info(f'删除本地临时压缩包成功 {callable_tar_result[2]}')
            except Exception as e:
                elog.warning(f'删除本地临时压缩包失败\n异常类型：\n{type(e)}\n异常信息：\n{e}')
                
        elog.info("execute_transfer传输任务结束，返回结果字典列表")
    
    return results






