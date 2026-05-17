# 三台云服务器搭建 K8s 集群（共享环境）

> 本指南搭建的 K8s 集群供 exp9、exp10、exp11、Lab4 共同使用。
> 每个实验使用独立 Namespace 隔离，互不干扰，可按需部署/卸载。

## 一、购买 ECS 实例

### 推荐配置

| 项目 | 规格 | 说明 |
|------|------|------|
| 实例规格 | ecs.g6.large | 2 vCPU + 8 GiB 内存 |
| 数量 | 3 台 | 1 master + 2 worker |
| 系统盘 | 40 GiB ESSD PL0 | Ubuntu 22.04 |
| 网络 | 同一 VPC 同一安全组 | 节点间互通 |
| 公网 | 每台绑定 EIP 或分配公网 IP | 方便 SSH 和 kubectl |
| 安全组 | 放行 6443, 10250, 30000-32767 | K8s 必需端口 |

### 安全组规则

```
方向  协议   端口范围      源地址          说明
入站  TCP    6443         0.0.0.0/0      K8s API Server
入站  TCP    10250        10.0.0.0/8     kubelet
入站  TCP    30000-32767  0.0.0.0/0      NodePort 服务
入站  TCP    22           0.0.0.0/0      SSH
入站  ICMP   -1           10.0.0.0/8     ping
入站  TCP    179          10.0.0.0/8     Calico BGP（如用 Calico）
入站  UDP    8472         10.0.0.0/8     Flannel VXLAN（如用 Flannel）
```

### 记录节点信息

```
master:   10.0.0.10  (公网 IP: x.x.x.x)
worker1:  10.0.0.11  (公网 IP: y.y.y.y)
worker2:  10.0.0.12  (公网 IP: z.z.z.z)
```

---

## 二、所有节点通用准备（三台都执行）

### 2.1 关闭 swap

```bash
sudo swapoff -a
sudo sed -i '/ swap / s/^\(.*\)$/#\1/g' /etc/fstab
```

### 2.2 配置内核参数

```bash
cat <<EOF | sudo tee /etc/sysctl.d/k8s.conf
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF
sudo sysctl --system
```

### 2.3 安装 containerd

```bash
# 安装 containerd
sudo apt-get update
sudo apt-get install -y containerd

# 生成默认配置
sudo mkdir -p /etc/containerd
containerd config default | sudo tee /etc/containerd/config.toml

# 使用 systemd cgroup driver（K8s 推荐）
sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml

# 重启 containerd
sudo systemctl restart containerd
sudo systemctl enable containerd
```

### 2.4 安装 kubeadm / kubelet / kubectl

```bash
# 添加 K8s apt 源
sudo apt-get install -y apt-transport-https ca-certificates curl gpg
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.28/deb/Release.key | sudo gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.28/deb/ /' | sudo tee /etc/apt/sources.list.d/kubernetes.list

# 安装
sudo apt-get update
sudo apt-get install -y kubelet kubeadm kubectl
sudo apt-mark hold kubelet kubeadm kubectl
```

---

## 三、初始化 Master 节点（仅 master 执行）

### 3.1 初始化集群

```bash
# 注意替换为 master 节点的内网 IP
sudo kubeadm init \
  --apiserver-advertise-address=10.0.0.10 \
  --pod-network-cidr=10.244.0.0/16 \
  --service-cidr=10.96.0.0/12

# 配置 kubectl
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
```

### 3.2 安装网络插件（Flannel）

```bash
kubectl apply -f https://github.com/flannel-io/flannel/releases/latest/download/kube-flannel.yml
```

### 3.3 安装 metrics-server（exp9 需要）

```bash
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

# 因为是自签名证书，需要添加 --kubelet-insecure-tls 参数
kubectl patch deployment metrics-server -n kube-system --type='json' -p='[
  {"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--kubelet-insecure-tls"}
]'
```

### 3.4 验证 Master

```bash
kubectl get nodes
# 应该看到 master 节点，状态可能暂时是 NotReady

kubectl get pods -n kube-system
# 等待所有 Pod 变为 Running
```

---

## 四、加入 Worker 节点（两台 worker 执行）

```bash
# 使用 kubeadm init 输出的 join 命令，类似：
sudo kubeadm join 10.0.0.10:6443 \
  --token <token> \
  --discovery-token-ca-cert-hash sha256:<hash>

# 如果 token 过期，在 master 上重新生成：
# kubeadm token create --print-join-command
```

### 验证集群就绪

```bash
# 在 master 上执行
kubectl get nodes
# NAME     STATUS   ROLES           AGE   VERSION
# master   Ready    control-plane   5m    v1.28.x
# worker1  Ready    <none>          2m    v1.28.x
# worker2  Ready    <none>          2m    v1.28.x

kubectl get pods -n kube-system
# 所有 Pod 应该都是 Running
```

---

## 五、配置 ACR 镜像仓库

### 5.1 开通容器镜像服务

1. 打开 https://cr.console.aliyun.com/
2. 开通个人版（免费）
3. 创建命名空间：`cloud-lab`
4. 设置 Registry 登录密码

### 5.2 所有节点配置 ACR 登录

```bash
# 在所有 3 台节点上执行
sudo apt-get install -y docker.io  # 仅用于登录 ACR
sudo docker login --username=<阿里云用户名> registry.cn-shanghai.aliyuncs.com
```

或者使用 containerd 直接拉取（推荐）：

```bash
# 在所有 3 台节点上配置 crictl
cat <<EOF | sudo tee /etc/crictl.yaml
runtime-endpoint: unix:///run/containerd/containerd.sock
image-endpoint: unix:///run/containerd/containerd.sock
timeout: 10
EOF
```

---

## 六、集群资源总览

部署所有实验后的资源使用情况：

| Namespace | 工作负载 | Pod 数 | CPU Request | Mem Request |
|-----------|---------|--------|-------------|-------------|
| exp9 | game-service (HPA 1→10) | 1~10 | 100m~1000m | 64Mi~640Mi |
| exp10 | faas-runtime | 1 | 100m | 128Mi |
| exp11 | game-service | 1 | 50m | 32Mi |
| battleworld | gateway + 3 nodes | 4 | 400m | 256Mi |
| **合计（基础）** | | **7** | **650m** | **480Mi** |
| **合计（exp9 扩满）** | | **16** | **1550m** | **1380Mi** |

> 3 台 2C8G 节点总资源：6 vCPU + 24 GiB，足够同时运行所有实验。
> exp9 HPA 扩容上限建议设为 6（每节点 2 个 Pod），避免资源争抢。

---

## 七、Namespace 规划

```bash
# 各实验使用独立 Namespace
kubectl create namespace exp9
kubectl create namespace exp10
kubectl create namespace exp11
kubectl create namespace battleworld    # Lab4

# 查看所有 Namespace
kubectl get namespaces
```

---

## 八、清理与重建

### 卸载单个实验

```bash
kubectl delete namespace exp9       # 清理 exp9
kubectl delete namespace exp10      # 清理 exp10
kubectl delete namespace exp11      # 清理 exp11
kubectl delete namespace battleworld # 清理 Lab4
```

### 完全重置集群

```bash
# 在所有节点执行
sudo kubeadm reset -f
sudo rm -rf /etc/cni /opt/cni /var/lib/cni /var/lib/kubelet /etc/kubernetes
sudo iptables -F && sudo iptables -t nat -F && sudo iptables -t mangle -F && sudo iptables -X

# 重新从 "三、初始化 Master" 开始
```

---

## 九、常用排错

```bash
# 节点 NotReady
kubectl describe node <node-name> | grep -A5 Conditions
journalctl -u kubelet -f

# Pod Pending
kubectl describe pod <pod-name> -n <namespace>
# 常见原因：资源不足、PVC 未绑定、镜像拉取失败

# 网络不通
kubectl exec -it <pod> -- ping <other-pod-ip>
kubectl get pods -n kube-system -l k8s-app=flannel

# metrics-server 不工作
kubectl top nodes
kubectl logs -n kube-system -l k8s-app=metrics-server
```
