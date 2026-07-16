# -*- coding: utf-8 -*-
# 测试密码文件验证功能

import os
import sys
import base64
import tempfile

# 添加项目路径
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from src.core import validate_password_file


def test_file_not_exists():
    """测试文件不存在场景"""
    print("测试：文件不存在...")
    try:
        validate_password_file("/nonexistent/path/password.txt")
        print("❌ 测试失败：应该抛出异常")
        return False
    except SystemExit:
        print("✓ 测试通过：文件不存在时正确退出")
        return True


def test_file_empty():
    """测试文件内容为空场景"""
    print("测试：文件内容为空...")
    with tempfile.NamedTemporaryFile(mode='w', delete=False, encoding='utf-8') as f:
        f.write("")
        temp_path = f.name

    try:
        validate_password_file(temp_path)
        print("❌ 测试失败：应该抛出异常")
        return False
    except SystemExit:
        print("✓ 测试通过：文件内容为空时正确退出")
        return True
    finally:
        os.unlink(temp_path)


def test_invalid_base64():
    """测试非 Base64 编码场景"""
    print("测试：非 Base64 编码...")
    with tempfile.NamedTemporaryFile(mode='w', delete=False, encoding='utf-8') as f:
        f.write("这不是Base64编码")
        temp_path = f.name

    try:
        validate_password_file(temp_path)
        print("❌ 测试失败：应该抛出异常")
        return False
    except SystemExit:
        print("✓ 测试通过：非 Base64 编码时正确退出")
        return True
    finally:
        os.unlink(temp_path)


def test_empty_decoded():
    """测试解码后为空场景"""
    print("测试：解码后为空...")
    # Base64 编码的空字符串
    empty_b64 = base64.b64encode(b"").decode('utf-8')
    with tempfile.NamedTemporaryFile(mode='w', delete=False, encoding='utf-8') as f:
        f.write(empty_b64)
        temp_path = f.name

    try:
        validate_password_file(temp_path)
        print("❌ 测试失败：应该抛出异常")
        return False
    except SystemExit:
        print("✓ 测试通过：解码后为空时正确退出")
        return True
    finally:
        os.unlink(temp_path)


def test_valid_password_file():
    """测试有效密码文件"""
    print("测试：有效密码文件...")
    password = "test_password_123"
    password_b64 = base64.b64encode(password.encode('utf-8')).decode('utf-8')
    with tempfile.NamedTemporaryFile(mode='w', delete=False, encoding='utf-8') as f:
        f.write(password_b64)
        temp_path = f.name

    try:
        validate_password_file(temp_path)
        print("✓ 测试通过：有效密码文件验证成功")
        return True
    except SystemExit:
        print("❌ 测试失败：有效密码文件不应该退出")
        return False
    finally:
        os.unlink(temp_path)


def main():
    """运行所有测试"""
    print("=" * 50)
    print("密码文件验证功能测试")
    print("=" * 50)

    tests = [
        test_file_not_exists,
        test_file_empty,
        test_invalid_base64,
        test_empty_decoded,
        test_valid_password_file,
    ]

    results = []
    for test in tests:
        results.append(test())
        print()

    print("=" * 50)
    print(f"测试结果：{sum(results)}/{len(results)} 通过")
    print("=" * 50)

    return all(results)


if __name__ == "__main__":
    success = main()
    sys.exit(0 if success else 1)