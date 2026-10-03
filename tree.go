// Package orgtree 提供进程内的下线组织树组件，
// 支持并发安全地迁移整棵下线子树，并按层查询团队成员。
//
// 并发语义：所有 Move / Team 调用在内部互斥锁保护下原子生效，
// 因此已完成的调用都可解释为在调用与返回之间某一时刻原子发生，
// 并遵守调用的先后关系（可线性化）。查询只会看到迁移前或迁移后的
// 完整结构，不会观察到中间态。
package orgtree

import "sync"

// maxTeamDepth 是 Team 查询的最大层数。
const maxTeamDepth = 10

// Tree 是一棵并发安全的下线组织树。
type Tree struct {
	mu      sync.RWMutex
	root    int64
	parent  map[int64]int64    // 每个非根节点 -> 直属上级
	members map[int64]struct{} // 全部节点集合（初始化后固定）
}

// New 根据分销商 ID 及直属上级 ID 构造组织树。
// records 的键为分销商 ID，值为其直属上级 ID；
// 根节点的上级 ID 约定为不在键集合中的值（例如 0）。
// 输入保证是一棵合法的单根树，ID 唯一。初始化后节点集合固定。
func New(records map[int64]int64) *Tree {
	t := &Tree{
		parent:  make(map[int64]int64, len(records)),
		members: make(map[int64]struct{}, len(records)),
	}
	for id, pid := range records {
		t.members[id] = struct{}{}
		if _, ok := records[pid]; ok {
			t.parent[id] = pid
		} else {
			t.root = id
		}
	}
	return t
}

// Move 将 member 及其全部下线迁到 newParent 下面，保持子树内部关系不变。
// 根节点不可迁移；newParent 处于 member 子树内（含 member 自身）时拒绝迁移。
// 目标已经是当前上级时视为成功的无操作。
// 返回迁移是否成功；失败时不留下任何关系变化。
func (t *Tree) Move(member, newParent int64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if member == t.root {
		return false
	}
	if t.parent[member] == newParent {
		return true // 无操作
	}
	// 环检测：沿 newParent 的上级链走到根，若途经 member 则成环。
	// 走整条链而非固定层数，树深可以超过 10 层。
	for cur := newParent; ; {
		if cur == member {
			return false
		}
		if cur == t.root {
			break
		}
		cur = t.parent[cur]
	}
	t.parent[member] = newParent
	return true
}

// Team 返回 member 以下第 1 至第 10 层的成员 ID 分组，不包含 member。
// 返回值下标 i 对应第 i+1 层；未达到 10 层时只返回实际存在的层，
// 不遗漏也不重复成员。
func (t *Tree) Team(member int64) [][]int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// 由 parent 映射临时构建孩子索引，避免在 Tree 上维护冗余结构。
	children := make(map[int64][]int64, len(t.parent))
	for id, pid := range t.parent {
		children[pid] = append(children[pid], id)
	}

	var team [][]int64
	level := []int64{member}
	for depth := 0; depth < maxTeamDepth && len(level) > 0; depth++ {
		var next []int64
		for _, id := range level {
			next = append(next, children[id]...)
		}
		if len(next) == 0 {
			break
		}
		team = append(team, next)
		level = next
	}
	return team
}
