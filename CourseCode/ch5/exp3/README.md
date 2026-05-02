# 实验三：一致性哈希演示

本实验在单进程命令行程序中演示一致性哈希的扩容过程，并对比两种上环方式：

- 物理节点直接上环：每个物理节点只在哈希环上占一个位置。
- 虚拟节点上环：每个真实节点拆成多个虚拟节点，再分布到哈希环上。

运行程序时可以输入节点数量、玩家数量等参数。程序会先输出扩容前的负载分布并生成 before 图，按回车加入新节点后，再输出扩容后的迁移情况并生成 after 图。

## 目录结构与文件作用（当前）

```text
exp3/
├── go.mod                                  # 独立模块：exp3
├── README.md                               # ← 本文件
├── cmd/                                    # 程序入口目录
│   ├── consistent_hash_physical/           # 物理节点直接上环版本
│   │   └── main.go                         # 构建物理节点哈希环，统计负载，计算扩容迁移并生成物理版图片
│   └── consistent_hash_virtual/            # 虚拟节点版本
│       └── main.go                         # 构建虚拟节点哈希环，统计负载，计算多段迁移并生成虚拟版图片
├── internal/                               # 实验内部复用代码
│   └── ringviz/                            # 哈希环 SVG 可视化工具
│       ├── physical.go                     # 物理节点版绘图：before/after 图，扩容后高亮迁移区间
│       └── virtual.go                      # 虚拟节点版绘图：before/after 图，保留迁移区间但不显示编号
└── images/                                 # 程序运行后生成的图片目录
    ├── physical/                           # 物理节点版图片输出目录
    │   ├── ring_physical_before.svg        # 物理节点版：扩容前哈希环
    │   └── ring_physical_after.svg         # 物理节点版：扩容后哈希环，高亮迁移区间
    └── virtual/                            # 虚拟节点版图片输出目录
        ├── ring_virtual_before.svg         # 虚拟节点版：扩容前哈希环
        └── ring_virtual_after.svg          # 虚拟节点版：扩容后哈希环，显示迁移区间弧线
```


## 运行方式

先进入实验目录：

```powershell
cd exp3
```

运行物理节点直接上环版本：

```powershell
go run ./cmd/consistent_hash_physical
```

运行后按提示输入：

- 初始节点数量，默认 `4`
- 玩家数量，默认 `100000`

程序会先输出初始节点负载分布，并生成：

```text
images/physical/ring_physical_before.svg
```

随后按回车加入新节点，程序会输出扩容后的重映射比例和迁移区间摘要，并生成：

```text
images/physical/ring_physical_after.svg
```

运行虚拟节点版本：

```powershell
go run ./cmd/consistent_hash_virtual
```

运行后按提示输入：

- 初始真实节点数量，默认 `4`
- 每个真实节点的虚拟节点数量，默认 `20`
- 玩家数量，默认 `100000`

程序会先输出初始负载分布，并生成：

```text
images/virtual/ring_virtual_before.svg
```

随后按回车加入新真实节点，程序会输出扩容后的重映射比例和迁移区间摘要，并生成：

```text
images/virtual/ring_virtual_after.svg
```

## 可视化说明

- 物理节点版中，蓝色节点表示原有物理节点，橙色节点表示新增物理节点，红色弧线表示扩容后由新节点接管的迁移区间。
- 虚拟节点版中，同一真实节点的虚拟节点使用同一种颜色，新增节点的虚拟节点使用橙色，扩容后仍会用红色弧线标出迁移区间。
- `before` 图用于观察扩容前的环结构和负载分布；`after` 图用于观察新增节点后哪些区间发生迁移。

## 对比观察

物理节点直接上环时，每个节点只占一个哈希位置，因此不同节点负责的区间可能差异很大，负载更容易倾斜。新增节点时，通常只接管一个连续区间。

虚拟节点上环时，每个真实节点被拆成多个虚拟节点，分散到哈希环不同位置。这样通常可以让负载更均匀；新增真实节点时，会出现多段较小的迁移区间，迁移更分散。

## 默认参数

- 物理节点版默认初始节点：`Node-0` 到 `Node-3`
- 虚拟节点版默认初始真实节点：`Node-0` 到 `Node-3`
- 默认新增节点：如果初始节点数量为 `N`，新增节点为 `Node-N`
- 默认玩家数量：`100000`
- 虚拟节点版默认每个真实节点的虚拟节点数量：`20`

## 最近修改内容

- 将原来的 `cmd/demo` 改名为 `cmd/consistent_hash_physical`。
- 将原来的 `cmd/demo_virtual` 改名为 `cmd/consistent_hash_virtual`。
- 将图片输出目录拆分为 `images/physical` 和 `images/virtual`。
- 虚拟节点版图片名称改为 `ring_virtual_before.svg` 和 `ring_virtual_after.svg`。
- 将虚拟节点绘图文件整理为 `internal/ringviz/virtual.go`。
- 虚拟节点版可视化调整为和物理节点版风格一致，并保留迁移区间弧线但去掉迁移区间编号。
