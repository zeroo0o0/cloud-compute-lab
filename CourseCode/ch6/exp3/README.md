# 实验三：进入容器排查问题

## 目标

- 进入 `game-service` 容器
- 查看进程、端口、网络、日志
- 验证容器内能够访问 `storage`

## 前置条件

先完成实验四，并发送一次请求生成日志：

```powershell
cd E:\work\cloud-compute-book-code\CourseCode\ch6\exp4
docker compose up -d
```

```powershell
@'
$client = [System.Net.Sockets.TcpClient]::new()
$client.Connect('127.0.0.1', 18080)
$stream = $client.GetStream()
$writer = New-Object System.IO.StreamWriter($stream)
$writer.NewLine = "`n"
$writer.AutoFlush = $true
$writer.WriteLine('MOVE demo-user d')
$reader = New-Object System.IO.StreamReader($stream)
while (($line = $reader.ReadLine()) -ne $null) {
  if ($line -eq '') { break }
  $line
}
$reader.Dispose(); $writer.Dispose(); $stream.Dispose(); $client.Dispose()
'@ | powershell
```

## 命令

交互进入：

```powershell
docker exec -it game-service /bin/sh
```

容器内执行：

```sh
ps aux
netstat -tlnp
ifconfig
ping -c 1 storage
cat /app/data/players.log
exit
```

单次执行：

```powershell
docker exec game-service ps aux
docker exec game-service netstat -tlnp
docker exec game-service ifconfig
docker exec game-service ping -c 1 storage
docker exec game-service cat /app/data/players.log
```

## 预期

- 可见 `game` 进程
- 可见 `8081` 监听
- `storage` 能解析并连通
- `players.log` 可读取
