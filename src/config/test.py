import json

with open('c:/Users/Administrator/Desktop/Code/sshexec_py/src/config/dangerous_keywords.json', 'r', encoding='utf-8') as f:
    data = json.load(f)
    print(data)
    print(type(data))
