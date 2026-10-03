// Package orgtree 提供进程内的下线组织树组件，
// 支持并发安全地迁移整棵下线子树、按层查询团队成员。
package orgtree

import "sync"

// Record 描述一个分销商节点及其直属上级。
// 父 ID 为 0（或不在记录集合中）的节点视为根节点。
type Record struct {
	ID       int64
	ParentID int64
}

// Tree 是一棵并发安全的下线组织树。
// 初始化后节点集合固定，只允许通过 Move 调整父子关系。
type Tree struct {
	mu    sync.RWMutex
	root  int64
	nodes map[int64]*node
}

type node struct {
	id       int64
	parent   int64 // 根节点的 parent 为 0
	children map[int64]struct{}
}

// New 根据分销商 ID 及直属上级 ID 构造组织树。
// 输入保证是一棵合法的单根树，ID 唯一。
func New(records []Record) *Tree {
	t := &Tree{nodes: make(map[int64]*node, len(records))}
	for _, r := range records {
		t.nodes[r.ID] = &node{id: r.ID, children: make(map[int64]struct{})}
	}
	for _, r := range records {
		n := t.nodes[r.ID]
		if _, ok := t.nodes[r.ParentID]; !ok {
			// 父节点不存在于记录中，视为根。
			t.root = r.ID
			continue
		}
		n.parent = r.ParentID
		t.nodes[r.ParentID].children[r.ID] = struct{}{}
	}
	return t
}

// Move 将 member 及其全部下线迁到 newParent 下面，保持子树内部关系不变。
// 根节点不可迁移；newParent 处于 member 子树内（含 member 自身）时拒绝。
// 目标已经是当前上级时视为成功的无操作。
// 返回迁移是否成功。
func (t *Tree) Move(member, newParent int64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	m := t.nodes[member]
	if member == t.root {
		return false
	}
	if m.parent == newParent {
		// 目标已是当前上级，视为成功的无操作。
		return true
	}
	// 环检测：沿 newParent 的父链一路走到根，
	// 若途中经过 member，说明 newParent 在 member 子树内，拒绝。
	// 该检测覆盖整棵树，与 Team 的 10 层限制无关。
	for cur := newParent; cur != 0; {
		if cur == member {
			return false
		}
		cur = t.nodes[cur].parent
	}
	// 摘挂整棵子树：只改 member 的父指针及两端的 children 集合，
	// 子树内部关系保持不变。
	delete(t.nodes[m.parent].children, member)
	m.parent = newParent
	t.nodes[newParent].children[member] = struct{}{}
	return true
}

// maxTeamDepth 是 Team 查询的最大层数。
const maxTeamDepth = 10

// Team 返回 member 以下第 1 至第 10 层的成员 ID 分组，不包含 member。
// 未达到 10 层时返回实际存在的层。返回下标 i 对应第 i+1 层。
// 整个查询在单次读锁内完成，结果对应某一时刻的完整快照。
func (t *Tree) Team(member int64) [][]int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var levels [][]int64
	current := []int64{member}
	for depth := 0; depth < maxTeamDepth && len(current) > 0; depth++ {
		var next []int64
		for _, id := range current {
			for childID := range t.nodes[id].children {
				next = append(next, childID)
			}
		}
		if len(next) == 0 {
			break
		}
		levels = append(levels, next)
		current = next
	}
	return levels
}
