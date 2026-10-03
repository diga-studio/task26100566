package orgtree

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

// chainRecords 构造 1->2->...->n 的链式树。
func chainRecords(n int64) []Record {
	recs := make([]Record, 0, n)
	for i := int64(1); i <= n; i++ {
		recs = append(recs, Record{ID: i, ParentID: i - 1})
	}
	return recs
}

// normalize 把每层 ID 排序，便于比较。
func normalize(levels [][]int64) [][]int64 {
	out := make([][]int64, len(levels))
	for i, lv := range levels {
		cp := append([]int64(nil), lv...)
		sort.Slice(cp, func(a, b int) bool { return cp[a] < cp[b] })
		out[i] = cp
	}
	return out
}

func equalLevels(a, b [][]int64) bool {
	a, b = normalize(a), normalize(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}
	return true
}

// checkInvariants 校验：从每个节点沿父链都能在有限步内到达根，
// 且 children 集合与 parent 指针互为一致（连通、无环、恰一个上级）。
func (t *Tree) checkInvariants(tb testing.TB) {
	tb.Helper()
	t.mu.RLock()
	defer t.mu.RUnlock()

	n := len(t.nodes)
	for id := range t.nodes {
		seen := map[int64]struct{}{}
		cur := id
		for cur != t.root {
			if _, dup := seen[cur]; dup {
				tb.Fatalf("节点 %d 的父链存在环", id)
			}
			seen[cur] = struct{}{}
			if len(seen) > n {
				tb.Fatalf("节点 %d 的父链超过节点总数，存在环", id)
			}
			p := t.nodes[cur].parent
			if p == 0 {
				tb.Fatalf("节点 %d 的父链未到达根，树不连通", id)
			}
			cur = p
		}
	}
	for id, nd := range t.nodes {
		for childID := range nd.children {
			if t.nodes[childID].parent != id {
				tb.Fatalf("节点 %d 的 children 含 %d，但 %d.parent=%d", id, childID, childID, t.nodes[childID].parent)
			}
		}
		if nd.parent != 0 {
			if _, ok := t.nodes[nd.parent].children[id]; !ok {
				tb.Fatalf("节点 %d 的 parent=%d，但 %d.children 不含 %d", id, nd.parent, nd.parent, id)
			}
		}
	}
}

func TestTeamBasic(t *testing.T) {
	//     1
	//    / \
	//   2   3
	//  / \   \
	// 4   5   6
	// |
	// 7
	tree := New([]Record{
		{ID: 1, ParentID: 0},
		{ID: 2, ParentID: 1},
		{ID: 3, ParentID: 1},
		{ID: 4, ParentID: 2},
		{ID: 5, ParentID: 2},
		{ID: 6, ParentID: 3},
		{ID: 7, ParentID: 4},
	})

	if got := tree.Team(1); !equalLevels(got, [][]int64{{2, 3}, {4, 5, 6}, {7}}) {
		t.Fatalf("Team(1) = %v", got)
	}
	if got := tree.Team(2); !equalLevels(got, [][]int64{{4, 5}, {7}}) {
		t.Fatalf("Team(2) = %v", got)
	}
	if got := tree.Team(7); len(got) != 0 {
		t.Fatalf("Team(7) 应为空, got %v", got)
	}
}

func TestTeamDepthCap(t *testing.T) {
	tree := New(chainRecords(15)) // 1->2->...->15

	got := tree.Team(1)
	if len(got) != 10 {
		t.Fatalf("Team(1) 应返回 10 层, got %d 层", len(got))
	}
	for i, lv := range got {
		if len(lv) != 1 || lv[0] != int64(i+2) {
			t.Fatalf("第 %d 层应为 [%d], got %v", i+1, i+2, lv)
		}
	}
	// 从第 6 层节点查询，只剩 9 层，不得补齐或遗漏。
	if got := tree.Team(6); len(got) != 9 {
		t.Fatalf("Team(6) 应返回 9 层, got %d 层", len(got))
	}
}

func TestMoveRules(t *testing.T) {
	build := func() *Tree {
		return New([]Record{
			{ID: 1, ParentID: 0},
			{ID: 2, ParentID: 1},
			{ID: 3, ParentID: 1},
			{ID: 4, ParentID: 2},
			{ID: 7, ParentID: 4},
		})
	}

	t.Run("根节点不可迁移", func(t *testing.T) {
		tree := build()
		if tree.Move(1, 3) {
			t.Fatal("迁移根节点应被拒绝")
		}
		tree.checkInvariants(t)
	})

	t.Run("迁到自己下面被拒绝", func(t *testing.T) {
		tree := build()
		if tree.Move(2, 2) {
			t.Fatal("Move(2,2) 应被拒绝")
		}
		tree.checkInvariants(t)
	})

	t.Run("迁到后代下面被拒绝", func(t *testing.T) {
		tree := build()
		if tree.Move(2, 4) {
			t.Fatal("Move(2,4) 应被拒绝")
		}
		if tree.Move(2, 7) {
			t.Fatal("Move(2,7) 应被拒绝")
		}
		tree.checkInvariants(t)
		// 拒绝后结构不变
		if got := tree.Team(1); !equalLevels(got, [][]int64{{2, 3}, {4}, {7}}) {
			t.Fatalf("拒绝后结构改变: %v", got)
		}
	})

	t.Run("目标已是当前上级为成功无操作", func(t *testing.T) {
		tree := build()
		if !tree.Move(2, 1) {
			t.Fatal("Move(2,1) 应为成功无操作")
		}
		if got := tree.Team(1); !equalLevels(got, [][]int64{{2, 3}, {4}, {7}}) {
			t.Fatalf("无操作后结构改变: %v", got)
		}
	})

	t.Run("正常迁移保持子树内部关系", func(t *testing.T) {
		tree := build()
		if !tree.Move(2, 3) {
			t.Fatal("Move(2,3) 应成功")
		}
		tree.checkInvariants(t)
		if got := tree.Team(1); !equalLevels(got, [][]int64{{3}, {2}, {4}, {7}}) {
			t.Fatalf("迁移后 Team(1) = %v", got)
		}
		// 子树内部关系不变
		if got := tree.Team(2); !equalLevels(got, [][]int64{{4}, {7}}) {
			t.Fatalf("迁移后 Team(2) = %v", got)
		}
	})
}

func TestMoveCycleDetectionBeyondTenLevels(t *testing.T) {
	// 链 1->2->...->15，目标在 member 子树内且距 member 超过 10 层。
	tree := New(chainRecords(15))
	if tree.Move(2, 13) {
		t.Fatal("Move(2,13) 应被拒绝（13 在 2 的子树内，相距 11 层）")
	}
	if tree.Move(2, 15) {
		t.Fatal("Move(2,15) 应被拒绝")
	}
	tree.checkInvariants(t)
	// 迁到子树外（超过 10 层的旁支）必须成功：先造旁支。
	recs := chainRecords(15)
	recs = append(recs, Record{ID: 100, ParentID: 1})
	tree2 := New(recs)
	if !tree2.Move(2, 100) {
		t.Fatal("Move(2,100) 应成功")
	}
	tree2.checkInvariants(t)
}

func TestConcurrentCrossMoveExactlyOneSucceeds(t *testing.T) {
	// A=2, B=3 最初互不隶属（都是根 1 的直属下级）。
	// 并发 Move(2,3) 与 Move(3,2)：必须恰有一次成功。
	for round := 0; round < 500; round++ {
		tree := New([]Record{
			{ID: 1, ParentID: 0},
			{ID: 2, ParentID: 1},
			{ID: 3, ParentID: 1},
		})
		var wg sync.WaitGroup
		start := make(chan struct{})
		results := make([]bool, 2)
		for i, pair := range [][2]int64{{2, 3}, {3, 2}} {
			wg.Add(1)
			go func(i int, member, parent int64) {
				defer wg.Done()
				<-start
				results[i] = tree.Move(member, parent)
			}(i, pair[0], pair[1])
		}
		close(start)
		wg.Wait()

		if results[0] == results[1] {
			t.Fatalf("第 %d 轮: 必须恰有一次成功, got %v", round, results)
		}
		tree.checkInvariants(t)
		// 最终结构必须与成功的那一次迁移一致。
		if results[0] { // 2 挂到 3 下
			if got := tree.Team(1); !equalLevels(got, [][]int64{{3}, {2}}) {
				t.Fatalf("第 %d 轮: 结构应为 1->3->2, got %v", round, got)
			}
		} else { // 3 挂到 2 下
			if got := tree.Team(1); !equalLevels(got, [][]int64{{2}, {3}}) {
				t.Fatalf("第 %d 轮: 结构应为 1->2->3, got %v", round, got)
			}
		}
	}
}

func TestConcurrentMoveAndTeam(t *testing.T) {
	// 结构: 1 之下有 2、3；2 之下有 10、11、12。
	// 迁移协程在两种合法结构间来回移动子树 2：
	//   配置X: 1->{2,3}, 2->{10,11,12}
	//   配置Y: 1->{3}, 3->{2}, 2->{10,11,12}
	// 查询协程不断 Team(1)/Team(3)，每次结果必须完整等于 X 或 Y 之一。
	build := func() *Tree {
		return New([]Record{
			{ID: 1, ParentID: 0},
			{ID: 2, ParentID: 1},
			{ID: 3, ParentID: 1},
			{ID: 10, ParentID: 2},
			{ID: 11, ParentID: 2},
			{ID: 12, ParentID: 2},
		})
	}
	team1X := [][]int64{{2, 3}, {10, 11, 12}}
	team1Y := [][]int64{{3}, {2}, {10, 11, 12}}
	team3X := [][]int64(nil)
	team3Y := [][]int64{{2}, {10, 11, 12}}

	tree := build()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	errCh := make(chan string, 16)

	// 迁移协程：X->Y->X->Y ...
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			target := int64(3)
			if i%2 == 1 {
				target = 1
			}
			if !tree.Move(2, target) {
				errCh <- fmt.Sprintf("Move(2,%d) 应成功", target)
				return
			}
		}
	}()

	// 查询协程
	for q := 0; q < 4; q++ {
		wg.Add(1)
		go func(q int) {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				select {
				case <-stop:
					return
				default:
				}
				got1 := tree.Team(1)
				if !equalLevels(got1, team1X) && !equalLevels(got1, team1Y) {
					errCh <- fmt.Sprintf("Team(1) 出现非法快照: %v", got1)
					return
				}
				got3 := tree.Team(3)
				if !equalLevels(got3, team3X) && !equalLevels(got3, team3Y) {
					errCh <- fmt.Sprintf("Team(3) 出现非法快照: %v", got3)
					return
				}
			}
		}(q)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		// 让查询跑一段时间后停止迁移。
		for i := 0; i < 8; i++ {
			select {
			case <-stop:
				return
			case <-time.After(5 * time.Millisecond):
			}
		}
		close(stop)
	}()

	wg.Wait()
	close(errCh)
	for msg := range errCh {
		t.Fatal(msg)
	}
	tree.checkInvariants(t)

	// 迁移全部结束后，把树固定到配置 Y，新查询必须反映新结构。
	for !tree.Move(2, 3) { // 若当前已在 3 下则为成功无操作
	}
	if got := tree.Team(1); !equalLevels(got, team1Y) {
		t.Fatalf("迁移完成后的 Team(1) 应反映新结构, got %v", got)
	}
}
