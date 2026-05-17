# 实验二：数据卷持久化

## 目标

- 将宿主机目录挂载到容器内
- 在 `/app/data/players.log` 写入日志
- 删除容器后保留宿主机日志文件

## 前置条件

先进入 `exp1` 目录构建单容器镜像：

```bash
cd /mnt/e/work/cloud-compute-book-code/CourseCode/ch6/exp1
docker build -f docker/env_demo.Dockerfile -t game-map0:v1.0 .
```

Windows PowerShell 等价命令：

```powershell
cd E:\work\cloud-compute-book-code\CourseCode\ch6\exp1
docker build -f docker/env_demo.Dockerfile -t game-map0:v1.0 .
```

## 步骤

以下步骤以 Linux / WSL 为主。

进入目录：

```bash
cd /mnt/e/work/cloud-compute-book-code/CourseCode/ch6/exp2
```

准备宿主机目录：

```bash
host_data="$(pwd)/data/game-logs"
mkdir -p "$host_data"
rm -f "$host_data/players.log"
```

移除旧容器：

```bash
docker rm -f game-0 >/dev/null 2>&1 || true
```

启动容器：

```bash
docker run -d -p 8081:8080 -v "$host_data:/app/data" --name game-0 game-map0:v1.0
```

发送一次登录请求生成日志：

```bash
curl "http://127.0.0.1:8081/login?player=demo-user"
```

查看宿主机日志：

```bash
cat "$host_data/players.log"
```

删除容器：

```bash
docker rm -f game-0
```

再次查看宿主机日志：

```bash
cat "$host_data/players.log"
```

## Windows PowerShell 补充

进入目录：

```powershell
cd E:\work\cloud-compute-book-code\CourseCode\ch6\exp2
```

准备宿主机目录：

```powershell
$hostData = Join-Path (Get-Location) 'data\game-logs'
New-Item -ItemType Directory -Force $hostData | Out-Null
Remove-Item (Join-Path $hostData 'players.log') -ErrorAction SilentlyContinue
```

移除旧容器：

```powershell
docker rm -f game-0 2>$null
```

启动容器：

```powershell
docker run -d -p 8081:8080 -v "${hostData}:/app/data" --name game-0 game-map0:v1.0
```

发送一次登录请求生成日志：

```powershell
curl.exe "http://127.0.0.1:8081/login?player=demo-user"
```

查看宿主机日志：

```powershell
Get-Content (Join-Path $hostData 'players.log')
```

删除容器：

```powershell
docker rm -f game-0
```

再次查看宿主机日志：

```powershell
Get-Content (Join-Path $hostData 'players.log')
```

## 预期

- `players.log` 已生成
- 删除 `game-0` 后，`players.log` 仍存在
