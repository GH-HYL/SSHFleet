# -*- coding: utf-8 -*-
# SSHFleet 打印结果文件
# 该文件负责格式化输出结果，包括打印到终端和导出到Excel文件

# Backward compatibility - all functions migrated to src/output/ submodules
from src.output.terminal import format_statistic_results_to_terminal
from src.output.report import format_statistic_results_to_report
from src.output.xlsx import format_output_to_xlsx, format_dict_list_to_xlsx
