# -*- coding: utf-8 -*-
# SSHFleet 主程序
# 该文件负责解析命令行参数、加载配置文件、执行任务、处理结果并输出到终端和Excel文件

# 项目目录/
# ├── sshfleet.py                        # 主程序
# └── src/                              # 源代码文件夹
#     ├── go/                           # go语言的执行器文件夹
#     │   ├── SSHFleet                 # 执行器（Linux可执行文件）
#     │   └── SSHFleet.exe             # 执行器（Windows可执行文件）
#     ├── transfer/                     # 传输模块
#     │   ├── transfer_precheck.py      # 传输预检查文件
#     │   ├── transfer_check.py         # 传输检查文件
#     │   ├── transfer_utils.py         # 传输工具文件
#     │   └── transfer.py               # 传输入口文件
#     ├── config/                       # 配置文件夹
#     │   ├── SSHFleet.yaml             # 配置文件
#     │   ├── dangerous_keywords.json   # 危险命令关键词文件
#     │   └── error_keywords.json       # 错误分类关键词文件    
#     ├── gotogo.py                     # 命令执行器入口
#     ├── check.py                      # 检查文件
#     ├── color.py                      # 颜色文件
#     ├── core.py                       # 核心代码文件
#     ├── output.py                     # 打印结果文件
#     ├── yaml.py                       # 读取配置文件
#     └── utils.py                      # 工具文件


# 系统或第三方模块
import os
import sys
import json
from src import yaml
from datetime import datetime

# 自定义模块
import src.core as core
import src.utils as utils
import src.check as check
import src.output as output
import src.color as color
from src.utils import tlog


def main():
    """主程序"""

    # 加载配置文件
    config_path = "src/config/SSHFleet.yaml"
    try:
        config = yaml.load_config(config_path)
    except Exception as e:
        print(
            f"\033[91m[ERROR]\033[0m\033[93m [function:load_config]\033[0m 加载配置文件失败\n异常类型：\n{type(e)}\n异常信息：\n{e}"
        )
        sys.exit(1)

    # 初始化工具日志
    try:
        utils.init_tool_logger(config.paths.logs.historys, config)
        tlog.debug(f"{ '-' * 50}分割线{ '-' * 50}")
        tlog.success("初始化工具日志成功")
    except Exception as e:
        print(
            f"\033[91m[ERROR]\033[0m\033[93m [function:init_tool_logger]\033[0m 初始化工具日志失败\n异常类型：\n{type(e)}\n异常信息：\n{e}"
        )

    tlog.info(f"SSHFleet工具开始执行，时间：{datetime.now()}")
    tlog.info(f"工作路径：{os.getcwd()}")
    tlog.info(f"原始命令行参数：{sys.argv}")
    tlog.info(
        f"日志目录：{config.paths.logs.historys}，工具日志文件名：{config.paths.logs.tool}"
    )
    tlog.debug(f"{ '-' * 20}SSHFleet工具 - 准备阶段{ '-' * 20}")

    # 检查代码文件是否都存在
    check.check_files_exist(config)
    tlog.success("检查代码文件存在性成功")

    # 获取危险命令分类正则关键字
    if os.path.exists(config.paths.jsons.dangerous_keywords):
        try:
            with open(
                config.paths.jsons.dangerous_keywords, "r", encoding="utf-8"
            ) as f:
                dangerous_keywords = json.load(f)
        except Exception as e:
            tlog.error(
                f"JSON文件{config.paths.jsons.dangerous_keywords}读取内容失败\n异常类型：\n{type(e)}\n异常信息：\n{e}"
            )
            utils.print_error_informantion_and_exit(
                "gotogo.go_to_go",
                f"JSON文件{config.paths.jsons.dangerous_keywords}读取内容失败\n异常类型：\n{type(e)}\n异常信息：\n{e}",
            )

    # 参数解析
    args = core.parse_args(config)
    tlog.success(f"参数解析成功,解析结果: {args}")

    # 参数合规性检查
    check.check_arguments(args)
    tlog.success("输入的参数合规性检查成功")

    # 执行打包历史记录
    if args.z:
        core.zip_latest_history(args, config)
        tlog.success("打包历史记录成功")
        tlog.info("SSHFleet工具已退出")
        sys.exit(0)

    # 执行危险字典内容检查
    check.check_dangerous_content(args, dangerous_keywords)
    tlog.success("[check] 执行危险字典内容检查成功")

    # 读取节点信息
    nodesinfos = core.read_nodes_infos(args.f, config)
    tlog.success("读取节点信息成功")

    # 参数信息确认
    tlog.info("开始进行参数信息确认")
    core.arguments_confirm(args, nodesinfos)
    tlog.info("用户已核实通过参数信息")

    tlog.debug(f"{'-' * 20}SSHFleet工具 - 准备结束{'-' * 20}\n")
    tlog.debug(f"{'=' * 30}SSHFleet工具 - 执行阶段{'=' * 30}")

    # 获取错误分类关键字
    if os.path.exists(config.paths.jsons.error_keywords):
        try:
            with open(config.paths.jsons.error_keywords, "r", encoding="utf-8") as f:
                error_keywords = json.load(f)
        except Exception as e:
            tlog.error(
                f"JSON文件{config.paths.exe.batch_tool_windows}读取内容失败\n异常类型：\n{type(e)}\n异常信息：\n{e}"
            )
            utils.print_error_informantion_and_exit(
                "gotogo.go_to_go",
                f"JSON文件{config.paths.exe.batch_tool_windows}读取内容失败\n异常类型：\n{type(e)}\n异常信息：\n{e}",
            )

    # “全局”开始时间计时
    global_start_time = datetime.now()
    tlog.info(f"全局开始时间: {global_start_time}")

    # 生成日志目录
    exec_log_dir = utils.create_exec_log_dir(args, config)
    tlog.success(f"生成日志目录成功: {exec_log_dir}")

    if args.c or args.s:
        tlog.info("开始执行命令")
        tlog.info("进入“执行”主执行器")
        import src.gotogo as gotogo
        final_results = gotogo.go_to_go(args, config, nodesinfos, exec_log_dir, error_keywords)
        tlog.success("go_to_go主执行器执行完成")

        # 进入transfer主执行器
    if args.u or args.d:
        # 初始化执行日志记录器
        tlog.info("开始传输文件")
        import src.transfer.transfer_precheck as transfer_precheck
        
        # 传输预检查
        transfer_command = transfer_precheck.transfer_precheck(args.u, args.p)

        if transfer_command:
            # 如果有值，表示是纯文本文件，交给“命令”执行器处理
            tlog.info("进入“执行”主执行器")
            print("提示：检测到上传目标是纯文本文件，使用批量上传命令")
            tlog.info("提示：检测到上传目标是纯文本文件，使用批量上传命令")
            import src.gotogo as gotogo
            final_results = gotogo.go_to_go(args, config, nodesinfos, exec_log_dir, error_keywords, transfer_command)
            tlog.success("go_to_go主执行器执行完成")
        else:
            # 如果是空值，表示不是纯文本文件，有二进制内容，交给“transfer”执行器处理
            tlog.info("进入transfer主执行器")
            tlog.debug(f"日志内容轮转到文件: {exec_log_dir}/{config.paths.logs.exec}")

            utils.init_execution_logger(exec_log_dir, config.paths.logs.exec)
            tlog.success("初始化执行日志记录器成功")
            
            import src.transfer.transfer as transfer
            final_results = transfer.execute_transfer(args, nodesinfos, error_keywords)
            tlog.success("transfer主执行器执行完成")


    # “全局”结束时间计时
    global_stop_time = datetime.now()
    tlog.info(
        f"全局结束时间: {global_stop_time}，全局执行时间: {round((global_stop_time - global_start_time).total_seconds(), 3)}秒"
    )

    print(f"{'═' * 60}")
    print(f"SSHFleet工具{color.COLOR_GREEN}正在整理{color.COLOR_RESET}......")
    tlog.debug(f"{'=' * 30}SSHFleet工具 - 执行结束{'=' * 30}\n")
    tlog.debug(f"{'-' * 20}SSHFleet工具 - 整理阶段{'-' * 20}")

    if not final_results:
        utils.print_error_informantion_and_exit(
            "main",
            f"{color.COLOR_RED}results结果未能正常生成，跳过整理阶段{color.COLOR_RESET}",
        )

    # 计算统计结果信息
    results_statistic = core.results_statistics(
        final_results, nodesinfos, args, global_start_time, global_stop_time
    )
    

    # 格式化统计结果信息输出到终端
    output.format_statistic_results_to_terminal(results_statistic)
    

    # 格式化统计结果信息输出到报告文件
    output.format_statistic_results_to_report(
        results_statistic, exec_log_dir, args, config
    )
    

    # 保存执行资源文件
    core.save_execute_resource_files(args, exec_log_dir, config)
    

    # 创建最新日志符号链接
    utils.create_latest_log_symlink(config)
    

    # 格式化终端输出到Excel文件（转换output.txt格式）
    if config.enable.output_to_xlsx:
        output.format_output_to_xlsx(exec_log_dir, config)
        

    # 输出results字典列表到xlsx文件
    if config.enable.results_to_xlsx:
        output.format_dict_list_to_xlsx(final_results, exec_log_dir, config)
        

    tlog.debug(f"{'-' * 20}SSHFleet工具 - 整理结束{'-' * 20}")

    # 退出SSHFleet工具
    print("SSHFleet工具执行结束，已退出")
    tlog.info("SSHFleet工具已退出")
    tlog.debug(f"{ '-' * 50}分割线{ '-' * 50}" + "\n" * 3)
    sys.exit(0)


if __name__ == "__main__":
    """
    程序入口
    """
    try:
        # 进入主函数
        main()
    except KeyboardInterrupt:
        print("用户手动中断执行")
