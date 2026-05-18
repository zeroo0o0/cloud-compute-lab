# 阿里云 K8s 集群搭建指南（v1.36.1 + containerd + Flannel）

> 本指南搭建的 K8s 集群供 ch6、Lab4 共同使用。
> 每个实验使用独立 Namespace 隔离，互不干扰，可按需部署/卸载。

---

## 一、环境信息

### 集群节点

| 主机名 | 角色 | 私网 IP | 公网 IP | 规格 |
|--------|------|---------|---------|------|
| k8s-a | master | 10.0.1.10 | 120.79.8.174 | Ubuntu 22.04 / 16G |
| k8s-b | worker | 10.0.2.10 | — | Ubuntu 22.04 / 16G |
| k8s-c | worker | 10.0.2.11 | — | Ubuntu 22.04 / 16G |
| k8s-d | worker | 10.0.2.12 | — | Ubuntu 22.04 / 16G |

### 集群配置

| 项目 | 值 |
|------|-----|
| K8s 版本 | v1.36.1 |
| 容器运行时 | containerd |
| 网络插件 | Flannel (VXLAN) |
| Pod CIDR | 10.244.0.0/16 |
| Service CIDR | 10.96.0.0/12 |
| DNS 域名 | cluster.local |
| VPC | 10.0.0.0/16 |

### 阿里云镜像仓库 (ACR)

| 项目 | 值 |
|------|-----|
| Registry 地址 | `crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com` |
| 命名空间 | `hnu-cloud-compute` |
| 镜像仓库示例 | `ch6-exp9-game`, `ch6-exp10-runtime`, `ch6-exp11-game`, `lab4-battleworld` |

### 安全组规则

VPC `10.0.0.0/16` 内网全通（已配置），满足 K8s 节点间通信需求。

外部访问需要放行：

```
方向  协议   端口范围      源地址          说明
入站  TCP    6443         0.0.0.0/0      K8s API Server
入站  TCP    30000-32767  0.0.0.0/0      NodePort 服务
入站  TCP    22           0.0.0.0/0      SSH
```

> 你当前的安全组配置已足够。VPC 内 `10.0.0.0/16` 全通涵盖了 kubelet(10250)、Flannel VXLAN(8472)、Calico BGP(179) 等所有节点间端口。

---

## 二、所有节点通用准备（四台都执行）

以 root 用户执行，或加 `sudo`。

### 2.1 关闭 swap

```bash
swapoff -a
sed -i '/ swap / s/^\(.*\)$/#\1/g' /etc/fstab
```

### 2.2 配置内核参数

```bash
cat <<EOF | tee /etc/sysctl.d/k8s.conf
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF
sysctl --system
```

### 2.3 安装并配置 containerd

```bash
# 安装
apt-get update
apt-get install -y containerd

# 生成默认配置
mkdir -p /etc/containerd
containerd config default | tee /etc/containerd/config.toml

# 使用 systemd cgroup driver（K8s 推荐）
sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml

# 修改 sandbox 镜像地址（registry.k8s.io 在国内超时）
# 将 registry.k8s.io/pause:3.10 替换为阿里云镜像
sed -i "s#sandbox = 'registry.k8s.io/pause:.*'#sandbox = 'registry.aliyuncs.com/google_containers/pause:3.10.2'#" /etc/containerd/config.toml
```

### 2.4 配置 containerd 镜像加速（关键步骤）

这一步解决 **registry.k8s.io** 和 **docker.io** 在国内拉取超时的问题。

```bash
# 备份原配置
cp /etc/containerd/config.toml /etc/containerd/config.toml.bak

# 修改 containerd 配置：将镜像源从默认的"内联配置"改为"目录配置"模式
# 原配置中 [plugins...registry.mirrors] 段定义了内联的镜像源，
# 我们需要把它替换为 config_path 模式，让 containerd 从 /etc/containerd/certs.d/ 目录读取镜像源
# 由于 config.toml 是嵌套 TOML 格式，sed 难以精确处理，这里用 Python 脚本

python3 << 'PYEOF'
import re

with open('/etc/containerd/config.toml', 'r') as f:
    content = f.read()

# 第一步：删除原有的 [plugins."io.containerd.grpc.v1.cri".registry] 整段
# （包括其下的 mirrors、configs 等子配置）
content = re.sub(
    r'\[plugins\."io\.containerd\.grpc\.v1\.cri"\.registry\].*?(?=\n\[|\Z)',
    '',
    content,
    flags=re.DOTALL
)

# 第二步：在文件末尾追加新的 registry 配置，指向 /etc/containerd/certs.d/ 目录
content += """
        [plugins."io.containerd.grpc.v1.cri".registry]
          config_path = "/etc/containerd/certs.d"
"""

with open('/etc/containerd/config.toml', 'w') as f:
    f.write(content)
PYEOF

# 验证修改结果（应该看到 config_path 那一行）
grep -A2 'registry' /etc/containerd/config.toml
```

创建各镜像源的 hosts 配置：

```bash
# === registry.k8s.io → 阿里云镜像 ===
mkdir -p /etc/containerd/certs.d/registry.k8s.io
cat > /etc/containerd/certs.d/registry.k8s.io/hosts.toml << 'EOF'
server = "https://registry.k8s.io"

[host."https://registry.aliyuncs.com/google_containers"]
  capabilities = ["pull", "resolve"]
  override_path = true
EOF

# === docker.io → 阿里云镜像 + 备用源 ===
mkdir -p /etc/containerd/certs.d/docker.io
cat > /etc/containerd/certs.d/docker.io/hosts.toml << 'EOF'
server = "https://docker.io"

[host."https://docker.m.daocloud.io"]
  capabilities = ["pull", "resolve"]

[host."https://docker.1panel.live"]
  capabilities = ["pull", "resolve"]

[host."https://mirror.ccs.tencentyun.com"]
  capabilities = ["pull", "resolve"]
EOF
```

重启 containerd 使配置生效：

```bash
systemctl restart containerd
systemctl enable containerd

# 验证镜像源配置
crictl pull registry.k8s.io/pause:3.10
# 如果输出 "Image is up to date" 或成功拉取，说明镜像源配置正确
# 如果仍然超时，见 "常见问题" 章节
```

### 2.5 安装 kubeadm / kubelet / kubectl

```bash
# 添加 K8s v1.36 apt 源
apt-get install -y apt-transport-https ca-certificates curl gpg
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.36/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.36/deb/ /' | tee /etc/apt/sources.list.d/kubernetes.list

# 安装
apt-get update
apt-get install -y kubelet kubeadm kubectl
apt-mark hold kubelet kubeadm kubectl
```

---

## 三、初始化 Master 节点（仅 k8s-a 执行）

### 3.1 编写 kubeadm 配置文件

```bash
cat > kubeadm-config.yaml << 'EOF'
apiVersion: kubeadm.k8s.io/v1beta4
kind: InitConfiguration
localAPIEndpoint:
  advertiseAddress: "10.0.1.10"
  bindPort: 6443
nodeRegistration:
  name: "k8s-a"
---
apiVersion: kubeadm.k8s.io/v1beta4
kind: ClusterConfiguration
kubernetesVersion: "v1.36.1"
imageRepository: "registry.aliyuncs.com/google_containers"
networking:
  podSubnet: "10.244.0.0/16"
  serviceSubnet: "10.96.0.0/12"
  dnsDomain: "cluster.local"
EOF
```

> `imageRepository` 设为 `registry.aliyuncs.com/google_containers`，
> kubeadm 会自动从此地址拉取 kube-apiserver、etcd、coredns 等组件镜像，
> 避免从 `registry.k8s.io` 拉取超时。

### 3.2 初始化集群

```bash
kubeadm init --config=kubeadm-config.yaml
```

**成功输出示例**：

```
Your Kubernetes control plane has initialized successfully!

To start using your cluster, you need to run the following as a regular user:

  mkdir -p $HOME/.kube
  sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
  sudo chown $(id -u):$(id -g) $HOME/.kube/config

Then you can join any number of control-plane nodes by running:

  kubeadm join 10.0.1.10:6443 --token <token> \
    --discovery-token-ca-cert-hash sha256:<hash> \
    --control-plane

Then you can join any number of worker nodes by running:

  kubeadm join 10.0.1.10:6443 --token <token> \
    --discovery-token-ca-cert-hash sha256:<hash>
```

**⚠️ 保存输出的 join 命令，后续 Worker 节点加入需要用到。**

### 3.3 配置 kubectl

```bash
mkdir -p $HOME/.kube
cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
chown $(id -u):$(id -g) $HOME/.kube/config

# 验证
kubectl get nodes
# NAME     STATUS     ROLES           AGE   VERSION
# k8s-a    NotReady   control-plane   10s   v1.36.1
```

> 状态为 `NotReady` 是正常的，还没装网络插件。

### 3.4 安装 Flannel 网络插件

```bash
# 下载 Flannel manifest
curl -fsSL https://github.com/flannel-io/flannel/releases/latest/download/kube-flannel.yml -o kube-flannel.yml

# 检查镜像地址（确认是否需要替换）
grep "image:" kube-flannel.yml
# 如果显示 docker.io/flannel/... 则 containerd 的 docker.io 镜像源会自动处理
# 如果仍然拉取超时，手动替换：
sed -i 's|docker.io/flannel/flannel:|registry.aliyuncs.com/google_containers/flannel:|g' kube-flannel.yml
sed -i 's|docker.io/flannel/flannel-cni-plugin:|registry.aliyuncs.com/google_containers/flannel-cni-plugin:|g' kube-flannel.yml

# 应用
kubectl apply -f kube-flannel.yml

# 等待 Flannel 就绪
kubectl -n kube-system wait --for=condition=ready pod -l app=flannel --timeout=120s

# 验证节点变为 Ready
kubectl get nodes
# NAME     STATUS   ROLES           AGE   VERSION
# k8s-a    Ready    control-plane   2m    v1.36.1
```

### 3.5 安装 metrics-server

```bash
# 下载 metrics-server manifest
curl -fsSL https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml -o metrics-server.yaml

# 查看镜像地址
grep "image:" metrics-server.yaml
# 通常是 registry.k8s.io/metrics-server/metrics-server:v0.x.x

# 替换为国内镜像（registry.k8s.io 在国内拉取超时）
# 注意：registry.aliyuncs.com 拉不到 metrics-server，必须用 registry.cn-hangzhou.aliyuncs.com
METRICS_IMAGE=$(grep "image:" metrics-server.yaml | head -1 | awk '{print $2}')
sed -i "s|${METRICS_IMAGE}|registry.cn-hangzhou.aliyuncs.com/google_containers/metrics-server:v0.8.1|" metrics-server.yaml

# 添加 --kubelet-insecure-tls 参数（kubeadm 自签名证书必须加）
sed -i '/- --metric-resolution=15s/a\        - --kubelet-insecure-tls' metrics-server.yaml

# 应用
kubectl apply -f metrics-server.yaml

# 等待 metrics-server 就绪
kubectl -n kube-system wait --for=condition=ready pod -l k8s-app=metrics-server --timeout=120s

# 验证
kubectl top nodes
# NAME     CPU(cores)   CPU%   MEMORY(bytes)   MEMORY%
# k8s-a    150m         7%     1200Mi          15%
```

---

## 四、加入 Worker 节点（k8s-b / k8s-c / k8s-d 执行）

### 4.1 加入集群

在每台 Worker 节点上执行 join 命令。**注意每台节点需要修改 `--node-name`**，这样在 `kubectl get nodes` 中能清楚看到每个节点的身份。

```bash
# === k8s-b 执行 ===
kubeadm join 10.0.1.10:6443 \
  --token <token> \
  --discovery-token-ca-cert-hash sha256:<hash> \
  --cri-socket=unix:///var/run/containerd/containerd.sock \
  --node-name=k8s-b

# === k8s-c 执行 ===
kubeadm join 10.0.1.10:6443 \
  --token <token> \
  --discovery-token-ca-cert-hash sha256:<hash> \
  --cri-socket=unix:///var/run/containerd/containerd.sock \
  --node-name=k8s-c

# === k8s-d 执行 ===
kubeadm join 10.0.1.10:6443 \
  --token <token> \
  --discovery-token-ca-cert-hash sha256:<hash> \
  --cri-socket=unix:///var/run/containerd/containerd.sock \
  --node-name=k8s-d
```

> 如果 token 已过期（默认 24 小时），在 master 上重新生成：
> ```bash
> kubeadm token create --print-join-command
> ```

### 4.2 验证集群就绪

```bash
# 在 master (k8s-a) 上执行
kubectl get nodes
# NAME     STATUS   ROLES           AGE   VERSION
# k8s-a    Ready    control-plane   10m   v1.36.1
# k8s-b    Ready    <none>          3m    v1.36.1
# k8s-c    Ready    <none>          2m    v1.36.1
# k8s-d    Ready    <none>          1m    v1.36.1

kubectl get pods -n kube-system
# 所有 Pod 应该都是 Running 或 Completed
```

---

## 五、配置 ACR 镜像仓库访问

### 5.1 创建 ACR ImagePullSecret

```bash
# 在 master 上创建 secret（用于拉取实验镜像）
# 这里的docker-server是我自己搭建的阿里云ACR个人仓库的地址，可以替换为你自己的地址
ACR_SERVER="crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com"
ACR_USER="<你的阿里云用户名>"
ACR_PASS="<你的ACR登录密码>"

# 为指定的 namespace 创建 secret
# 用法：先创建 namespace，再创建 secret
# 按需修改下方的 namespace 列表
for ns in exp9 exp10 exp11 Lab4; do
  kubectl create namespace $ns 2>/dev/null || true
  kubectl create secret docker-registry acr-secret \
    --namespace=$ns \
    --docker-server=$ACR_SERVER \
    --docker-username=$ACR_USER \
    --docker-password=$ACR_PASS
done
```

### 5.2 验证 ACR 访问

```bash
# 测试拉取镜像（在任意节点执行）
crictl pull crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/ch6-exp9-game:latest
```

### 5.3 在 K8s 中使用 ACR 镜像

Deployment 中需要引用 imagePullSecret：

部分配置如下：
```yaml
spec:
  template:
    spec:
      imagePullSecrets:
      - name: acr-secret
      containers:
      - name: xxx
        image: crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/ch6-exp9-game:latest
        imagePullPolicy: Always
```

---

## 六、集群资源总览

部署所有实验后的资源使用情况：

| Namespace | 工作负载 | Pod 数 | CPU Request | Mem Request |
|-----------|---------|--------|-------------|-------------|
| exp9 | game-service (HPA 1→6) | 1~6 | 100m~600m | 64Mi~384Mi |
| exp10 | faas-runtime | 1 | 100m | 128Mi |
| exp11 | game-service | 1 | 50m | 32Mi |
| Lab4 | gateway + 3 nodes | 4 | 400m | 256Mi |
| **合计（基础）** | | **7** | **650m** | **480Mi** |
| **合计（exp9 扩满）** | | **12** | **1150m** | **880Mi** |

> 4 台 16G 节点总资源充足，所有实验可同时运行。

---

## 七、Namespace 规划

```bash
# 各实验使用独立 Namespace
kubectl create namespace exp9
kubectl create namespace exp10
kubectl create namespace exp11
kubectl create namespace Lab4

# 查看所有 Namespace
kubectl get namespaces
```

---

## 八、清理与重建

### 卸载单个实验

```bash
kubectl delete namespace exp9
kubectl delete namespace exp10
kubectl delete namespace exp11
kubectl delete namespace Lab4
```

### 完全重置集群

```bash
# 在所有节点执行
kubeadm reset -f
rm -rf /etc/cni /opt/cni /var/lib/cni /var/lib/kubelet /etc/kubernetes
iptables -F && iptables -t nat -F && iptables -t mangle -F && iptables -X

# 重新从 "三、初始化 Master" 开始
```

---

## 九、常见问题与踩坑记录

### 9.1 kubeadm init 卡在 [wait-control-plane] 超时

**现象**：`kubeadm init` 长时间卡住，最终报 `[wait-control-plane] Couldn't initialize a Kubernetes cluster`

**原因**：kube-apiserver 等镜像从 `registry.k8s.io` 拉取超时

**解决**：

```bash
# 确认 containerd 镜像源配置是否生效
cat /etc/containerd/certs.d/registry.k8s.io/hosts.toml

# 手动测试拉取
crictl pull registry.k8s.io/pause:3.10
# 如果仍然超时，检查 containerd 日志
journalctl -u containerd -f

# 备选方案：使用 kubeadm config images list 查看所需镜像，手动从阿里云拉取并 tag
kubeadm config images list --kubernetes-version=v1.36.1
# 输出类似：
# registry.k8s.io/kube-apiserver:v1.36.1
# registry.k8s.io/kube-controller-manager:v1.36.1
# registry.k8s.io/kube-scheduler:v1.36.1
# registry.k8s.io/kube-proxy:v1.36.1
# registry.k8s.io/coredns/coredns:v1.12.0
# registry.k8s.io/etcd:3.6.1-0
# registry.k8s.io/pause:3.10

# 手动从阿里云拉取并修改 tag
for img in kube-apiserver:v1.36.1 kube-controller-manager:v1.36.1 kube-scheduler:v1.36.1 kube-proxy:v1.36.1 etcd:3.6.1-0 pause:3.10; do
  crictl pull registry.aliyuncs.com/google_containers/$img
  ctr -n k8s.io images tag registry.aliyuncs.com/google_containers/$img registry.k8s.io/$img
done

# coredns 需要特殊处理（路径带 coredns/）
crictl pull registry.aliyuncs.com/google_containers/coredns:v1.12.0
ctr -n k8s.io images tag registry.aliyuncs.com/google_containers/coredns:v1.12.0 registry.k8s.io/coredns/coredns:v1.12.0
```

### 9.2 Flannel Pod 一直 Init:ImagePullBackOff

**现象**：`kubectl get pods -n kube-system` 显示 flannel Pod 状态为 `Init:ImagePullBackOff`

**原因**：`docker.io/flannel/flannel` 和 `docker.io/flannel/flannel-cni-plugin` 在国内拉取超时

**解决**：

```bash
# 方案 A：containerd docker.io 镜像源生效（已配置在 2.4 节）
# 检查配置
cat /etc/containerd/certs.d/docker.io/hosts.toml

# 方案 B：手动拉取并替换
# 查看 flannel 需要的镜像
grep "image:" kube-flannel.yml

# 手动从备用源拉取
crictl pull docker.m.daocloud.io/flannel/flannel:v0.26.7
crictl pull docker.m.daocloud.io/flannel/flannel-cni-plugin:v1.6.2-flannel1

# 重新打 tag
ctr -n k8s.io images tag docker.m.daocloud.io/flannel/flannel:v0.26.7 docker.io/flannel/flannel:v0.26.7
ctr -n k8s.io images tag docker.m.daocloud.io/flannel/flannel-cni-plugin:v1.6.2-flannel1 docker.io/flannel/flannel-cni-plugin:v1.6.2-flannel1

# 删除 flannel pod 让它重新创建
kubectl -n kube-system delete pod -l app=flannel
```

### 9.3 metrics-server Pod 一直 CrashLoopBackOff

**现象**：`kubectl top nodes` 报错 `metrics not available yet`，metrics-server Pod 反复重启

**原因 1**：镜像拉取超时

```bash
# 检查 Pod 状态
kubectl -n kube-system describe pod -l k8s-app=metrics-server

# 如果是 ImagePullBackOff，手动拉取
crictl pull registry.cn-hangzhou.aliyuncs.com/google_containers/metrics-server:v0.8.1
ctr -n k8s.io images tag registry.cn-hangzhou.aliyuncs.com/google_containers/metrics-server:v0.8.1 registry.k8s.io/metrics-server/metrics-server:v0.8.1
```

**原因 2**：kubelet 自签名证书导致 metrics-server 无法采集指标

```bash
# 检查 metrics-server 日志
kubectl -n kube-system logs -l k8s-app=metrics-server | grep -i "tls\|certificate\|x509"

# 确认 --kubelet-insecure-tls 参数已添加
kubectl -n kube-system get deployment metrics-server -o jsonpath='{.spec.template.spec.containers[0].args}'
# 应该包含 --kubelet-insecure-tls
```

**原因 3**：Pod 未就绪就开始采集

```bash
# 等待 metrics-server 完全就绪
kubectl -n kube-system wait --for=condition=ready pod -l k8s-app=metrics-server --timeout=180s
# metrics-server 首次启动可能需要 1-2 分钟
```

### 9.4 Worker 节点 kubeadm join 超时

**现象**：Worker 执行 `kubeadm join` 时超时

**原因**：Worker 无法访问 master 的 6443 端口

**解决**：

```bash
# 在 worker 上测试连通性
curl -k https://10.0.1.10:6443/healthz
# 应该返回 "ok"

# 如果不通，检查：
# 1. 安全组是否放行了 6443 端口
# 2. master 的防火墙
iptables -L -n | grep 6443
# 如果没有规则，添加：
iptables -A INPUT -p tcp --dport 6443 -j ACCEPT
```

### 9.5 containerd 配置修改后不生效

**现象**：修改了 `/etc/containerd/config.toml` 但镜像拉取仍然走原来的源

**解决**：

```bash
# 必须重启 containerd
systemctl restart containerd

# 验证配置是否正确
containerd config dump | grep -A10 "registry"

# 如果使用 certs.d 方式，确认目录结构
tree /etc/containerd/certs.d/
# /etc/containerd/certs.d/
# ├── docker.io/
# │   └── hosts.toml
# └── registry.k8s.io/
#     └── hosts.toml
```

### 9.6 coredns Pod 一直 Pending 或 CrashLoopBackOff

**现象**：`kubectl get pods -n kube-system` 显示 coredns 状态异常

**原因**：通常是因为 Flannel 网络插件未就绪

**解决**：

```bash
# 先确认 Flannel 正常
kubectl -n kube-system get pods -l app=flannel

# 如果 Flannel 正常但 coredns 仍然异常，重启 coredns
kubectl -n kube-system delete pod -l k8s-app=kube-dns
```

---

## 十、常用排错命令速查

```bash
# 节点状态
kubectl get nodes -o wide
kubectl describe node <node-name>

# 所有系统 Pod
kubectl get pods -n kube-system -o wide

# 查看 Pod 详情（排错首选）
kubectl -n kube-system describe pod <pod-name>

# kubelet 日志
journalctl -u kubelet -f --lines=50

# containerd 日志
journalctl -u containerd -f --lines=50

# 检查镜像是否已拉取
crictl images

# 手动拉取镜像
crictl pull <image>

# 检查节点资源
kubectl top nodes
kubectl top pods -A

# 检查网络连通性
kubectl run debug --image=busybox --rm -it --restart=Never -- ping 10.244.0.1
```
