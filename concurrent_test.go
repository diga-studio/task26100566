package orgtree

import (
	"sync"
	"testing"
)

// 场景一：A 与 B 最初互不隶属，并发 Move(A,B) 与 Move(B,A)。
// 必须恰有一次成功，另一次因成环被拒绝，最终结构与结果一致。
func TestConcurrentConflictingMoves(t *testing.T) {
	for round := 0; round < 200; round++ {
		// 根 1 下两个互不隶属的分支：A=2（带 4），B=3（带 5）。
		tr := New(map[int64]int64{
			1: 0,
			2: 1, 4: 2,
			3: 1, 5: 3,
		})

		var wg sync.WaitGroup
		res := make([]bool, 2)
		start := make(chan struct{})
		for i, op := range [2][2]int64{{2, 3}, {3, 2}} {
			wg.Add(1)
			go func(i int, member, parent int64) {
				defer wg.Done()
				<-start
				res[i] = tr.Move(member, parent)
			}(i, op[0], op[1])
		}
		close(start)
		wg.Wait()

		if res[0] == res[1] {
			t.Fatalf("round %d: 必须恰有一次成功, got %v", round, res)
		}

		// 验证最终结构与成功的迁移一致。
		if res[0] { // Move(2,3) 成功：2 挂到 3 下
			assertTeam(t, tr, 1, [][]int64{{3}, {5, 2}, {4}})
			assertTeam(t, tr, 3, [][]int64{{5, 2}, {4}})
		} else { // Move(3,2) 成功：3 挂到 2 下
			assertTeam(t, tr, 1, [][]int64{{2}, {4, 3}, {5}})
			assertTeam(t, tr, 2, [][]int64{{4, 3}, {5}})
		}
	}
}

// 场景二：并发执行跨分支的 Move 与 Team。
// 查询结果必须完整对应迁移前或迁移后某一合法时刻，
// 不得出现重复成员、成员消失或层级混杂。
func TestConcurrentMoveAndTeam(t *testing.T) {
	for round := 0; round < 100; round++ {
		// 1
		// ├── 2 ── 4 ── 6
		// └── 3 ── 5
		tr := New(map[int64]int64{
			1: 0,
			2: 1, 4: 2, 6: 4,
			3: 1, 5: 3,
		})
		all := []int64{2, 3, 4, 5, 6}

		// 迁移前 Team(1) 的合法快照。
		before := [][]int64{{2, 3}, {4, 5}, {6}}
		// Move(2,5) 后 Team(1) 的合法快照。
		after := [][]int64{{3}, {5}, {2}, {4}, {6}}

		start := make(chan struct{})
		var wg sync.WaitGroup

		// 一个写者：跨分支迁移 2 -> 5。
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if !tr.Move(2, 5) {
				t.Error("Move(2,5) 应成功")
			}
		}()

		// 多个读者并发查询。
		errs := make(chan string, 64)
		for r := 0; r < 8; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for i := 0; i < 50; i++ {
					team := tr.Team(1)
					if msg := checkSnapshot(team, before, after, all); msg != "" {
						errs <- msg
						return
					}
				}
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for msg := range errs {
			t.Fatalf("round %d: %s", round, msg)
		}

		// 迁移完成后的新查询必须反映新结构。
		assertTeam(t, tr, 1, after)
		assertTeam(t, tr, 5, [][]int64{{2}, {4}, {6}})
	}
}

// checkSnapshot 校验 team 是 before 或 after 之一的合法快照，
// 且成员无重复、无消失、无层级混杂。
func checkSnapshot(team, before, after [][]int64, all []int64) string {
	if !equalTeam(team, before) && !equalTeam(team, after) {
		return "观察到非法中间态: " + sprintTeam(team)
	}
	seen := make(map[int64]int) // id -> 层
	for depth, level := range team {
		for _, id := range level {
			if _, dup := seen[id]; dup {
				return "成员重复出现: id 出现在多层"
			}
			seen[id] = depth
		}
	}
	for _, id := range all {
		if _, ok := seen[id]; !ok {
			return "成员消失"
		}
	}
	return ""
}

func equalTeam(a, b [][]int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalSet(a[i], b[i]) {
			return false
		}
	}
	return true
}

func equalSet(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	cnt := make(map[int64]int, len(a))
	for _, id := range a {
		cnt[id]++
	}
	for _, id := range b {
		cnt[id]--
		if cnt[id] < 0 {
			return false
		}
	}
	return true
}

func sprintTeam(team [][]int64) string {
	s := "["
	for i, level := range team {
		if i > 0 {
			s += " "
		}
		s += "{"
		for j, id := range level {
			if j > 0 {
				s += ","
			}
			s += string(rune('0' + id))
		}
		s += "}"
	}
	return s + "]"
}

func assertTeam(t *testing.T, tr *Tree, member int64, want [][]int64) {
	t.Helper()
	got := tr.Team(member)
	if !equalTeam(got, want) {
		t.Fatalf("Team(%d) = %v, want %v", member, got, want)
	}
}
