# Legacy reference script kept for reverse-engineering comparison only.
# /// script
# requires-python = ">=3.12"
# dependencies = [
#     "requests",
#     "beautifulsoup4",
#     "typer",
#     "rich",
# ]
# ///

import requests
import json
import time
import typer
from typing import Dict, Tuple
from bs4 import BeautifulSoup
from rich.console import Console
from rich.table import Table

app = typer.Typer(help="NJUPT 校园网资源迁移工具")
console = Console()

class NJUPTSelfService:
    BASE_URL = "http://10.10.244.240:8080/Self"
    
    def __init__(self, username, password):
        self.username = username
        self.password = password
        self.session = requests.Session()
        self.session.headers.update({
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
            "Upgrade-Insecure-Requests": "1"
        })

    def login(self):
        try:
            login_url = f"{self.BASE_URL}/login/?302=LI"
            resp = self.session.get(login_url, timeout=10)
            soup = BeautifulSoup(resp.text, 'html.parser')
            checkcode = soup.find('input', {'name': 'checkcode'})['value']
            self.session.get(f"{self.BASE_URL}/login/randomCode?t={time.time()}", timeout=10)
            
            data = {"foo": "", "bar": "", "checkcode": checkcode, "account": self.username, "password": self.password, "code": ""}
            headers = {"Referer": login_url, "Origin": "http://10.10.244.240:8080", "Content-Type": "application/x-www-form-urlencoded"}
            
            resp = self.session.post(f"{self.BASE_URL}/login/verify", data=data, headers=headers, allow_redirects=False, timeout=10)
            if resp.status_code == 302 and "dashboard" in resp.headers.get("Location", ""):
                self.session.get(f"{self.BASE_URL}/dashboard", timeout=10)
                return True
            return False
        except: return False

    def get_binding(self):
        """获取当前绑定的移动账号"""
        try:
            resp = self.session.get(f"{self.BASE_URL}/service/operatorId", timeout=10)
            soup = BeautifulSoup(resp.text, 'html.parser')
            acc = soup.find('input', {'name': 'FLDEXTRA3'})
            return acc.get('value', "") if acc else ""
        except: return None

    def set_binding(self, acc="", pw=""):
        """设置绑定（空为解绑）"""
        url = f"{self.BASE_URL}/service/operatorId"
        try:
            resp = self.session.get(url, timeout=10)
            soup = BeautifulSoup(resp.text, 'html.parser')
            csrftoken = soup.find('input', {'name': 'csrftoken'})['value']
            data = {"csrftoken": csrftoken, "FLDEXTRA1": "", "FLDEXTRA2": "", "FLDEXTRA3": acc, "FLDEXTRA4": pw, "FLDEXTRA5": "", "FLDEXTRA6": ""}
            # 保持其他运营商原样
            for i in [1, 2, 5, 6]:
                field = f"FLDEXTRA{i}"
                inp = soup.find('input', {'name': field})
                if inp and inp.get('value'): data[field] = inp['value']
            
            headers = {"Referer": url, "Origin": "http://10.10.244.240:8080"}
            self.session.post(f"{self.BASE_URL}/service/bind-operator", data=data, headers=headers, timeout=15)
            time.sleep(0.5)
            return self.get_binding() == acc
        except: return False

def load_data():
    with open("credentials.json", "r") as f: return json.load(f)

def normalize_accounts(data: dict) -> Dict[str, dict]:
    """兼容两种配置结构，统一返回账号字典。"""
    if 'accounts' in data and isinstance(data['accounts'], dict):
        return data['accounts']
    if 'users' in data and isinstance(data['users'], dict):
        return data['users']
    raise KeyError("配置中缺少 accounts/users")

def resolve_target_key(accounts: Dict[str, dict], target: str) -> str | None:
    """优先精确匹配，失败后做大小写不敏感匹配。"""
    if target in accounts:
        return target
    lower_target = target.lower()
    for key in accounts:
        if key.lower() == lower_target:
            return key
    return None

def normalize_cmcc(data: dict, target_key: str, accounts: Dict[str, dict]) -> Tuple[dict, str]:
    """统一提取移动宽带配置。返回 (cmcc配置, 宽带标签)。"""
    if 'cmcc' in data and isinstance(data['cmcc'], dict):
        return data['cmcc'], 'cmcc'

    if 'broadbands' in data and isinstance(data['broadbands'], dict):
        user = accounts[target_key]
        bind_bb = user.get('bind_broadband', '')
        if not bind_bb:
            raise KeyError(f"账号 '{target_key}' 未配置 bind_broadband")
        bb = data['broadbands'].get(bind_bb)
        if not isinstance(bb, dict):
            raise KeyError(f"配置中不存在 broadband '{bind_bb}'")
        return bb, bind_bb

    raise KeyError("配置中缺少 cmcc/broadbands")

@app.command()
def status():
    """查看所有账号的当前绑定状态"""
    data = load_data()
    accounts = normalize_accounts(data)
    table = Table(title="NJUPT 校园网资源状态")
    table.add_column("标签", style="bold cyan")
    table.add_column("账号", style="dim")
    table.add_column("移动宽带绑定", style="magenta")
    
    for tag, creds in accounts.items():
        client = NJUPTSelfService(creds['username'], creds['password'])
        if client.login():
            bound = client.get_binding()
            table.add_row(tag, creds['username'], bound or "[dim]未绑定[/]")
        else:
            table.add_row(tag, creds['username'], "[red]登录失败[/]")
    console.print(table)

@app.command()
def move(target: str = typer.Argument(..., help="目标账号标签 (如 W 或 B)")):
    """自动将宽带资源迁移到指定账号"""
    data = load_data()
    accounts = normalize_accounts(data)
    target_key = resolve_target_key(accounts, target)
    if not target_key:
        console.print(f"[red]错误: 找不到标签为 '{target}' 的账号[/]")
        return

    try:
        cmcc, bb_tag = normalize_cmcc(data, target_key, accounts)
    except KeyError as e:
        console.print(f"[red]错误: {e}[/]")
        return

    console.print(f"🚀 [bold blue]启动资源迁移流程 -> 目标: {target_key} (宽带: {bb_tag})[/]")

    # 1. 扫描所有账号，找到“资源占有者”
    holder_tag = None
    for tag, creds in accounts.items():
        if tag == target_key:
            continue
        client = NJUPTSelfService(creds['username'], creds['password'])
        if client.login():
            if client.get_binding() == cmcc['account']:
                holder_tag = tag
                break
    
    if holder_tag:
        console.print(f"  [1/2] 发现资源被 [yellow]{holder_tag}[/] 占用，正在释放...")
        holder_creds = accounts[holder_tag]
        h_client = NJUPTSelfService(holder_creds['username'], holder_creds['password'])
        h_client.login()
        if h_client.set_binding("", ""):
            console.print(f"    ✅ [green]资源已从 {holder_tag} 释放[/]")
        else:
            console.print(f"    ❌ [red]释放失败！[/]")
            return
    else:
        console.print(f"  [1/2] ℹ️ 未发现资源冲突，直接开始绑定。")

    # 2. 绑定到目标
    console.print(f"  [2/2] 正在为 [cyan]{target_key}[/] 绑定资源...")
    target_creds = accounts[target_key]
    t_client = NJUPTSelfService(target_creds['username'], target_creds['password'])
    if t_client.login():
        if t_client.set_binding(cmcc['account'], cmcc['password']):
            console.print(f"  🎉 [bold green]迁移成功！资源现在属于 {target_key}。[/]")
        else:
            console.print(f"  ❌ [red]绑定验证失败。[/]")
    else:
        console.print(f"  ❌ [red]目标账号登录失败。[/]")

if __name__ == "__main__":
    app()
