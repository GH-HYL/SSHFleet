# -*- coding: utf-8 -*-
# SSHFleet 主程序
# 该文件负责解析命令行参数、加载配置文件、执行任务、处理结果并输出到终端和Excel文件

# 项目目录/
# ├── sshfleet.py                        # 主程序
# └── src/                              # 源代码文件夹
#     ├── input/                        # 输入处理模块
#     │   ├── args.py                   #   命令行参数解析
#     │   ├── csv.py                    #   CSV 节点文件读取
#     │   └── confirm.py                #   参数信息交互确认
#     ├── check/                        # 校验模块
#     │   ├── arguments.py              #   参数合规性检查
#     │   ├── dangerous.py              #   危险命令检测
#     │   └── files.py                  #   文件存在性检查
#     ├── command/                      # 命令构建模块
#     │   └── builder.py                #   最终执行命令构建
#     ├── gotogo/                       # Go 执行器模块
#     │   ├── go_to_go.py               #   主执行函数
#     │   ├── caller.py                 #   Go 进程调用与 HTTP SSE 通信
#     │   ├── builder.py                #   请求体构建（命令/上传）
#     │   ├── parser.py                 #   SSE 响应解析
#     │   └── classifier.py             #   错误分类
#     ├── output/                       # 输出处理模块
#     │   ├── terminal.py               #   终端格式化输出
#     │   ├── report.py                 #   执行报告生成
#     │   ├── xlsx.py                   #   Excel 文件生成
#     │   ├── statistics.py             #   结果统计计算
#     │   └── archive.py                #   资源文件备份与打包
#     ├── log/                          # 日志模块
#     │   └── logger.py                 #   日志初始化与管理
#     ├── config/                       # 配置文件夹
#     │   ├── SSHFleet.yaml             #   工具配置
#     │   ├── dangerous_keywords.json   #   危险命令检测规则
#     │   └── error_keywords.json       #   错误分类关键词
#     ├── utils.py                      # 工具函数、装饰器
#     ├── yaml.py                       # 配置文件加载（Pydantic）
#     └── color.py                      # 终端颜色常量


# 系统或第三方模块
import os
import sys
import json
from datetime import datetime

# 自定义模块 - 新模块路径
from src.input.args import parse_args
from src.input.csv import read_nodes_infos
from src.input.confirm import arguments_confirm
from src.log.logger import tlog, init_tool_logger, create_exec_log_dir, create_latest_log_symlink
from src.check.arguments import check_arguments
from src.check.dangerous import check_dangerous_content
from src.check.files import check_files_exist
from src.output.statistics import results_statistics
from src.output.archive import save_execute_resource_files, zip_latest_history
from src.output.terminal import format_statistic_results_to_terminal
from src.output.report import format_statistic_results_to_report
from src.output.xlsx import format_output_to_xlsx, format_dict_list_to_xlsx
from src.gotogo.go_to_go import go_to_go
from src import yaml
import src.utils as utils
import src.color as color


def _load_json(path):
    """读取JSON文件并返回解析后的数据"""
    try:
        with open(path, "r", encoding="utf-8") as f:
            return json.load(f)
    except Exception as e:
        tlog.error(
            f"JSON文件{path}读取内容失败\n异常类型：\n{type(e)}\n异常信息：\n{e}"
        )
        utils.print_error_information_and_exit(
            "_load_json",
            f"JSON文件{path}读取内容失败\n异常类型：\n{type(e)}\n异常信息：\n{e}",
        )


def main():
    """主程序"""

    # 加载配置文件
    config_path = "src/config/SSHFleet.yaml"
    try:
        config = yaml.load_config(config_path)
    except Exception as e:
        print(
            f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:load_config]{color.COLOR_RESET} 加载配置文件失败\n异常类型：\n{type(e)}\n异常信息：\n{e}"
        )
        sys.exit(1)

    # 初始化工具日志
    try:
        init_tool_logger(config.paths.logs.historys, config)
        tlog.debug(f"{ '=' * 100}分割线{ '=' * 100}")
        tlog.debug(f"{ '=' * 100}分割线{ '=' * 100}")
        tlog.success("初始化工具日志成功")
    except Exception as e:
        print(
            f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:init_tool_logger]{color.COLOR_RESET} 初始化工具日志失败\n异常类型：\n{type(e)}\n异常信息：\n{e}"
        )

    tlog.info(f"SSHFleet工具开始执行，时间：{datetime.now()}")
    tlog.info(f"工作路径：{os.getcwd()}")
    tlog.info(f"原始命令行参数：{sys.argv}")
    tlog.info(
        f"日志目录：{config.paths.logs.historys}，工具日志文件名：{config.paths.logs.tool}"
    )
    tlog.debug(f"{ '-' * 20}SSHFleet工具 - 准备阶段{ '-' * 20}")

    # 检查代码文件是否都存在
    check_files_exist(config)
    tlog.success("检查代码文件存在性成功")

    # 获取危险命令分类正则关键字
    dangerous_keywords = _load_json(config.paths.jsons.dangerous_keywords)

    # 参数解析
    args = parse_args(config)
    tlog.success(f"参数解析成功,解析结果: {args}")

    # 参数合规性检查
    check_arguments(args)
    tlog.success("输入的参数合规性检查成功")

    # 执行打包历史记录
    if args.z:
        zip_latest_history(args, config)
        tlog.success("打包历史记录成功")
        tlog.info("SSHFleet工具已退出")
        sys.exit(0)

    # 执行危险字典内容检查
    check_dangerous_content(args, dangerous_keywords)
    tlog.success("[check] 执行危险字典内容检查成功")

    # 读取节点信息
    nodesinfos = read_nodes_infos(args.f, config, is_inline=getattr(args, 'f_is_inline', False))
    tlog.success("读取节点信息成功")

    # 参数信息确认
    tlog.info("开始进行参数信息确认")
    arguments_confirm(args, nodesinfos, config)
    tlog.info("用户已核实通过参数信息")

    tlog.debug(f"{'-' * 20}SSHFleet工具 - 准备结束{'-' * 20}\n")
    tlog.debug(f"{'-' * 30}SSHFleet工具 - 执行阶段{'-' * 30}")

    # 获取错误分类关键字
    error_keywords = _load_json(config.paths.jsons.error_keywords)

    # "全局"开始时间计时
    global_start_time = datetime.now()
    tlog.info(f"全局开始时间: {global_start_time}")

    # 生成日志目录
    exec_log_dir = create_exec_log_dir(args, config)
    tlog.success(f"生成日志目录成功: {exec_log_dir}")

    # 执行命令、脚本或上传（合并所有执行模式）
    if args.c or args.s or args.u:
        tlog.info("开始执行任务")
        tlog.info("进入主执行器")
        final_results = go_to_go(args, config, nodesinfos, exec_log_dir, error_keywords)
        tlog.success("go_to_go主执行器执行完成")

    # "全局"结束时间计时
    global_stop_time = datetime.now()
    tlog.info(
        f"全局结束时间: {global_stop_time}，全局执行时间: {round((global_stop_time - global_start_time).total_seconds(), 3)}秒"
    )

    print(f"{'═' * 60}")
    # print(f"SSHFleet工具{color.COLOR_GREEN}正在整理{color.COLOR_RESET}......")
    tlog.debug(f"{'-' * 30}SSHFleet工具 - 执行结束{'-' * 30}\n")
    tlog.debug(f"{'-' * 20}SSHFleet工具 - 整理阶段{'-' * 20}")

    if not final_results:
        utils.print_error_information_and_exit(
            "main",
            f"{color.COLOR_RED}results结果未能正常生成，跳过整理阶段{color.COLOR_RESET}",
        )

    # 计算统计结果信息
    results_stat = results_statistics(
        final_results, nodesinfos, args, global_start_time, global_stop_time
    )

    # 格式化统计结果信息输出到终端
    format_statistic_results_to_terminal(results_stat)

    # 格式化统计结果信息输出到报告文件
    format_statistic_results_to_report(
        results_stat, exec_log_dir, args, config
    )

    # 保存执行资源文件
    save_execute_resource_files(args, exec_log_dir, config)

    # 创建最新日志符号链接
    create_latest_log_symlink(config)

    # 格式化结构化数据到 output.xlsx（3列格式：IP、事件类型、内容详情）
    if config.enable.output_to_xlsx:
        format_output_to_xlsx(final_results, exec_log_dir, args, config)

    # 输出results字典列表到xlsx文件
    if config.enable.results_to_xlsx:
        format_dict_list_to_xlsx(final_results, exec_log_dir, config)

    tlog.debug(f"{'-' * 20}SSHFleet工具 - 整理结束{'-' * 20}")

    # 退出SSHFleet工具
    print("SSHFleet工具执行结束，已退出")
    tlog.info("SSHFleet工具已退出")
    tlog.debug(f"{ '=' * 100}分割线{ '=' * 100}")
    tlog.debug(f"{ '=' * 100}分割线{ '=' * 100}" + "\n" * 3)
    sys.exit(0)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("用户手动中断执行")
    except Exception as e:
        print(
            f"{color.COLOR_RED}[FATAL]{color.COLOR_RESET} 未捕获异常: {e}",
            file=sys.stderr,
        )
        sys.exit(1)
