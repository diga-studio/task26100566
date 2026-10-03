# task26100566

进程内并发安全的下线组织树组件（Go 库，包名 `orgtree`）。

## 接口

```go
// 根据分销商 ID 及直属上级 ID 构造组织树。
// 输入须为合法单根树、ID 唯一；ParentID 不在记录中的节点视为根。
// 初始化后节点集合固定。
func New(records []Record) *Tree

// 将 member 及整棵下线子树迁到 newParent 下，子树内部关系不变。
// 根节点不可迁移；newParent 在 member 子树内（含 member 自身）时拒绝；
// newParent 已是当前上级时为成功无操作。返回是否成功。
func (t *Tree) Move(member, newParent int64) bool

// 返回 member 以下第 1~10 层的成员 ID 分组（不含 member），
// 下标 i 对应第 i+1 层；不足 10 层时只返回实际存在的层。
func (t *Tree) Team(member int64) [][]int64
```

## 并发语义

- `Move` 持写锁、`Team` 持读锁，二者均可并发调用。
- 每次 `Move`/`Team` 都满足可线性化：结果等价于在调用与返回之间某一时刻原子发生。
- `Team` 在单次读锁内完成 BFS，快照要么是迁移前、要么是迁移后的完整结构，绝不拼接中间态。
- 环检测沿父链走到根，覆盖整棵树，不受 10 层查询上限影响；失败的迁移不留下任何关系变化。

## 测试

```sh
go test -race ./...
```

覆盖：分层查询与 10 层上限、全部迁移规则、超过 10 层的环检测、
并发 `Move(A,B)`/`Move(B,A)` 恰一次成功、并发 `Move` 与 `Team` 的快照一致性。
