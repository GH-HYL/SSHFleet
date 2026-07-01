# -*- mode: python ; coding: utf-8 -*-

block_cipher = None

a = Analysis(
    ['sshfleet.py'],
    pathex=[],
    binaries=[],
    datas=[
        ('src/config/SSHFleet.yaml', 'src/config'),
        ('src/config/dangerous_keywords.json', 'src/config'),
        ('src/config/error_keywords.json', 'src/config'),
    ],
    hiddenimports=[
        # src 模块
        'src',
        'src.core',
        'src.utils',
        'src.check',
        'src.output',
        'src.color',
        'src.yaml',
        # gotogo 模块
        'src.gotogo',
        'src.gotogo.builder',
        'src.gotogo.caller',
        'src.gotogo.classifier',
        'src.gotogo.go_to_go',
        'src.gotogo.parser',
        # transfer 模块
        'src.transfer',
        'src.transfer.transfer_router',
        'src.transfer.transfer',
        'src.transfer.transfer_precheck',
        'src.transfer.transfer_check',
        'src.transfer.transfer_utils',
        # 第三方依赖
        'fabric',
        'paramiko',
        'yaml',
        'pydantic',
        'rich',
        'rich.console',
        'rich.progress',
        'rich.live',
        'openpyxl',
        'loguru',
        'requests',
    ],
    hookspath=[],
    hooksconfig={},
    runtime_hooks=[],
    excludes=[],
    win_no_prefer_redirects=False,
    win_private_assemblies=False,
    cipher=block_cipher,
    noarchive=False,
)

pyz = PYZ(a.pure, a.zipped_data, cipher=block_cipher)

exe = EXE(
    pyz,
    a.scripts,
    [],
    exclude_binaries=True,
    name='SSHFleet',
    debug=False,
    bootloader_ignore_signals=False,
    strip=False,
    upx=True,
    console=True,
    disable_windowed_traceback=False,
)

coll = COLLECT(
    exe,
    a.binaries,
    a.zipfiles,
    a.datas,
    strip=False,
    upx=True,
    upx_exclude=[],
    name='SSHFleet',
)
