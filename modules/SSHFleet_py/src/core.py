# -*- coding: utf-8 -*-
# SSHFleet 核心文件
# 该文件负责定义核心函数和类，包括参数解析、配置加载、任务执行等

import argparse
import os
import re
import shutil

# 系统或第三方模块
import sys
from typing import Dict, List
from collections import Counter
from datetime import datetime
from pathlib import Path
from posixpath import join as posix_join

# 自定义模块
import src.color as color
import src.utils as utils
from src.yaml import SSHFleetConfig
from src.utils import tlog

# 向后兼容导入 - 从新的 input 模块导入已迁移的函数
from src.input import parse_args, validate_password_file, read_nodes_infos, arguments_confirm


@utils.error_and_exit_handling_decorator(
    "save_execute_resource_files", "保存执行资源文件失败", isexit=True
)
def save_execute_resource_files(
    args: argparse.Namespace, log_dir: str, config: SSHFleetConfig
) -> None:
    """
    功能：
        保存执行资源文件（脚本、上传文件、配置文件）到指定目录。

    参数：
        args: 命令行参数
        log_dir: 日志目录路径
        config: 配置对象

    返回：
        None
    """
    # 创建资源存放目录
    resources_dir = posix_join(log_dir, config.paths.files.asset)
    os.makedirs(resources_dir, exist_ok=True)

    # 处理 -s 参数（单个文件）
    if hasattr(args, "s") and args.s:
        src_path = Path(args.s)
        if src_path.exists():
            dst_path = Path(resources_dir) / src_path.name
            shutil.copy2(src_path, dst_path)  # 保留元数据复制

    # 处理 -f 参数（单个文件）
    if hasattr(args, "f") and args.f:
        src_path = Path(args.f)
        if src_path.exists():
            dst_path = Path(resources_dir) / src_path.name
            shutil.copy2(src_path, dst_path)
    tlog.success("保存执行资源文件成功")

@utils.error_and_exit_handling_decorator(
    "results_statistics", "计算统计结果信息失败", isexit=True
)
def results_statistics(
    results: List,
    node_infos: List,
    args: argparse.Namespace,
    global_start_time: datetime,
    global_stop_time: datetime,
) -> Dict:
    """
    功能：
        计算统计结果信息

    参数：
        results: 结果列表
        node_infos: 节点信息列表
        args: 命令行参数
        global_start_time: 全局开始时间
        global_stop_time: 全局结束时间

    返回：
        包含所有统计结果信息的字典
    """

    # 基本统计
    results_total = len(results)
    nodeinofs_total = len(node_infos)

    # 成功/失败统计
    exit_counts = Counter(d.get("exit_bool") for d in results)
    success_counts = exit_counts.get(True, 0)
    fail_counts = exit_counts.get(False, 0)

    # 总数校验
    verify = "通过" if nodeinofs_total == results_total else "异常"
    verify_color = (
        color.COLOR_GREEN if nodeinofs_total == results_total else color.COLOR_RED
    )

    # 分类统计
    category_counts = Counter(d.get("result_category") for d in results)

    # 根据模式移除成功分类
    if args.u:
        success_category = "传输成功"
        category_counts.pop(success_category, None)
    elif args.c or args.s:
        success_category = "执行成功"
        category_counts.pop(success_category, None)

    # 按数量正（倒）序排序（reverse逆转）失败分类
    sorted_fail_categories = sorted(
        category_counts.items(), key=lambda x: x[1], reverse=True
    )

    # 按分类分组并收集IP地址
    category_ip_map = {}
    for result in results:
        category = result.get("result_category", "未知")
        ip = result.get("ip", "未知IP")
        if category not in category_ip_map:
            category_ip_map[category] = []
        category_ip_map[category].append(ip)

    # 分离成功分类IP
    if "success_category" not in locals():
        tlog.error("success_category 未定义，未知执行模式")
        success_category = "未知"
    success_ips = category_ip_map.pop(success_category, [])

    # 对每个分类的IP进行数字排序
    for category in category_ip_map:
        category_ip_map[category] = sorted(
            category_ip_map[category],
            key=lambda ip: [int(part) for part in ip.split(".")],
        )

    # 对成功IP进行数字排序
    sorted_success_ips = (
        sorted(success_ips, key=lambda ip: [int(part) for part in ip.split(".")])
        if success_ips
        else []
    )

    # 构建统计结果字典
    statistics = {
        "results_total": results_total,
        "nodeinofs_total": nodeinofs_total,
        "verify": verify,
        "verify_color": verify_color,
        "success_counts": success_counts,
        "fail_counts": fail_counts,
        "sorted_fail_categories": sorted_fail_categories,
        "success_category": (
            success_category if "success_category" in locals() else None
        ),
        "success_ips_count": len(success_ips),
        "sorted_success_ips": sorted_success_ips,
        "category_ip_map": category_ip_map,
        "global_start_time": global_start_time,
        "global_stop_time": global_stop_time,
        "global_cost_time": round(
            (global_stop_time - global_start_time).total_seconds(), 2
        ),
    }
    tlog.success("计算统计结果信息成功")

    return statistics


@utils.error_and_exit_handling_decorator("zip_latest_history", "打包历史记录失败")
def zip_latest_history(args, config: SSHFleetConfig):
    """
    功能：
        打包最新的历史记录日志文件为 ZIP 或 TAR 格式

    参数：
        args: 命令行参数

    返回：
        None
    """

    history_dir = config.paths.logs.historys
    zip_dir = config.paths.logs.zip

    # -z 参数需要单独使用，与所有其他参数互斥
    if args.z:
        if len(sys.argv) > 2:
            print(
                f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:zip_latest_history]{color.COLOR_RESET} -z 模式不能与其他参数一起使用",
                file=sys.stderr,
            )
            sys.exit(1)

    if not os.path.isdir(history_dir):
        print(
            f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:zip_latest_history]{color.COLOR_RESET} 历史记录目录 '{history_dir}' 不存在",
            file=sys.stderr,
        )
        print("提示: 请先至少一次执行任务以生成历史记录")
        sys.exit(1)

    log_dirs = [
        d for d in os.listdir(history_dir) if os.path.isdir(posix_join(history_dir, d))
    ]
    if not log_dirs:
        print(
            f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:zip_latest_history]{color.COLOR_RESET} 历史记录目录 '{history_dir}' 中没有日志文件夹",
            file=sys.stderr,
        )
        print("提示: 请先至少一次执行任务以生成历史记录")
        sys.exit(1)

    # 按目录名排序，假设目录名格式为 log-YYYYMMDD_HHMMSS
    log_dirs.sort(reverse=True, key=lambda x: x.replace("_", ""))
    latest_log_dir = log_dirs[0]

    # 拼接压缩包名称
    working_dir = os.getcwd()
    package_name = f"log-{latest_log_dir}"

    # 如果配置文件为空或不存在，打包路径存放在工作路径下，过存在，检查是绝对路径还是相对路径，按照路径配置生成
    if zip_dir:
        # 格式化路径，确保使用正斜杠
        default_zip_dir = utils.args_normalize_path(zip_dir)

        # 检查是否是绝对路径
        if os.path.isabs(default_zip_dir):
            zip_dir = default_zip_dir
        else:
            zip_dir = posix_join(working_dir, default_zip_dir)
    else:
        zip_dir = working_dir

    # 检查打包路径是否存在，不存在则创建
    if not os.path.exists(zip_dir):
        utils.get_user_confirmation(
            f"{zip_dir} \n 配置文件的默认打包路径不存在，是否创建？", yorn=True
        )
        os.makedirs(zip_dir)

    zip_path = posix_join(zip_dir, f"{package_name}.zip")
    tar_path = posix_join(zip_dir, f"{package_name}.tar")

    # 筛选已存在的 ZIP 文件并删除
    zip_pattern = re.compile(r"log-\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2}_.*\.zip")
    existing_zip_files = [f for f in os.listdir(zip_dir) if zip_pattern.match(f)]
    for f in existing_zip_files:
        file_path = posix_join(zip_dir, f)
        os.remove(file_path)
        print(f"已清理当前路径下旧打包文件 {f}")

    # 筛选已存在的 TAR 文件并删除
    tar_pattern = re.compile(r"log-\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2}_.*\.tar")
    existing_tar_files = [f for f in os.listdir(zip_dir) if tar_pattern.match(f)]
    for f in existing_tar_files:
        file_path = posix_join(zip_dir, f)
        os.remove(file_path)
        print(f"已清理当前路径下旧打包文件 {f}")

    # 尝试使用 ZIP 打包
    try:
        import zipfile

        with zipfile.ZipFile(zip_path, "w", zipfile.ZIP_STORED) as zipf:
            source_dir = posix_join(history_dir, latest_log_dir)
            for root, _, files in os.walk(source_dir):
                for file in files:
                    file_path = posix_join(root, file)
                    arcname = os.path.relpath(file_path, source_dir)
                    zipf.write(file_path, arcname)
        print(f"ZIP打包成功: {os.path.basename(zip_path)}")
        return
    except Exception as e:
        print(f"ZIP打包失败: {str(e)}", file=sys.stderr)

    # ZIP 打包失败，尝试使用 TAR 打包
    try:
        import tarfile

        source_dir = posix_join(history_dir, latest_log_dir)
        with tarfile.open(tar_path, "w") as tar:
            tar.add(source_dir, arcname=os.path.basename(source_dir))
        print(f"TAR打包成功: {os.path.basename(tar_path)}")
    except Exception as e:
        print(f"TAR打包失败: {str(e)}", file=sys.stderr)
