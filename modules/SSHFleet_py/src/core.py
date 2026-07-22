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

# 向后兼容导入 - 从新的 output 模块导入已迁移的函数
from src.output import results_statistics, save_execute_resource_files, zip_latest_history
