# -*- coding: utf-8 -*-
# SSHFleet 检查文件
# 该文件负责检查文件是否存在、是否可写等操作

# Backward compatibility - all functions migrated to src/check/ submodules
from src.check.arguments import check_arguments
from src.check.dangerous import (
    check_dangerous_content,
    check_dangerous_dict,
    check_dangerous_patterns,
    print_danger_warning,
)
from src.check.files import check_files_exist, check_script_file
