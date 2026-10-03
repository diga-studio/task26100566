package orgtree

import (
	"reflect"
	"testing"
)

// 构建如下固定测试树（深度 12，超过 Team 的 10 层上限）：
//
//	1
//	└── 2
//	    ├── 3
//	    │   └── 6
//	    │       └── 8
//	    │           └── 10
//	    │               └── 12
//	    ├── 4
//	    │   └── 7
//	    ├── 5
//	    └── 9
//	        └── 11
func newFixture() *Tree {
	return New(map[int64]int64{
		1:  0,
		2:  1,
		3:  2,
		4:  2,
		5:  2,
		6:  3,
		7:  4,
		8:  6,
		9:  2,
		10: 8,
		11: 9,
		12: 10,
	})
}

func sorted(ids []int64) []int64 {
	out := append([]int64(nil), ids...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func TestNewAndTeam(t *testing.T) {
	tr := newFixture()

	got := tr.Team(2)
	want := [][]int64{
		{3, 4, 5, 9}, // 第 1 层
		{6, 7, 11},   // 第 2 层
		{8},          // 第 3 层
		{10},         // 第 4 层
		{12},         // 第 5 层
	}
	if len(got) != len(want) {
		t.Fatalf("Team(2) 层数 = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if !reflect.DeepEqual(sorted(got[i]), want[i]) {
			t.Fatalf("Team(2) 第 %d 层 = %v, want %v", i+1, sorted(got[i]), want[i])
		}
	}

	if got := tr.Team(12); len(got) != 0 {
		t.Fatalf("Team(12) = %v, want 空", got)
	}
}

func TestTeamDepthCapAt10(t *testing.T) {
	// 构造 15 层链：1->2->...->15，Team(1) 只能返回 10 层。
	records := map[int64]int64{1: 0}
	for i := int64(2); i <= 15; i++ {
		records[i] = i - 1
	}
	tr := New(records)
	got := tr.Team(1)
	if len(got) != 10 {
		t.Fatalf("Team(1) 层数 = %d, want 10", len(got))
	}
	for i, level := range got {
		want := []int64{int64(i + 2)}
		if !reflect.DeepEqual(level, want) {
			t.Fatalf("Team(1) 第 %d 层 = %v, want %v", i+1, level, want)
		}
	}
}

func TestMoveBasic(t *testing.T) {
	tr := newFixture()

	// 把 3（带 6,8,10,12）迁到 5 下面。
	if !tr.Move(3, 5) {
		t.Fatal("Move(3,5) 应成功")
	}
	got := tr.Team(5)
	want := [][]int64{{3}, {6}, {8}, {10}, {12}}
	if len(got) != len(want) {
		t.Fatalf("Team(5) 层数 = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if !reflect.DeepEqual(sorted(got[i]), want[i]) {
			t.Fatalf("Team(5) 第 %d 层 = %v, want %v", i+1, sorted(got[i]), want[i])
		}
	}
	// 2 的第 1 层不再包含 3。
	for _, id := range tr.Team(2)[0] {
		if id == 3 {
			t.Fatal("迁移后 3 不应再出现在 Team(2) 第 1 层")
		}
	}
}

func TestMoveRejected(t *testing.T) {
	tr := newFixture()
	before := tr.Team(1)

	cases := []struct {
		name              string
		member, newParent int64
	}{
		{"根节点不可迁移", 1, 5},
		{"迁到自己下面", 3, 3},
		{"迁到直接下线下面", 3, 6},
		{"迁到深层下线下面", 3, 12},
	}
	for _, c := range cases {
		if tr.Move(c.member, c.newParent) {
			t.Fatalf("%s: Move(%d,%d) 应被拒绝", c.name, c.member, c.newParent)
		}
	}
	// 失败的迁移不得留下任何关系变化。
	after := tr.Team(1)
	if len(after) != len(before) {
		t.Fatalf("拒绝迁移后结构变化: %v -> %v", before, after)
	}
	for i := range before {
		if !reflect.DeepEqual(sorted(before[i]), sorted(after[i])) {
			t.Fatalf("拒绝迁移后第 %d 层变化: %v -> %v", i+1, before[i], after[i])
		}
	}
}

func TestMoveNoopWhenSameParent(t *testing.T) {
	tr := newFixture()
	if !tr.Move(3, 2) {
		t.Fatal("目标已是当前上级时应视为成功无操作")
	}
	if got := tr.Team(2)[0]; !reflect.DeepEqual(sorted(got), []int64{3, 4, 5, 9}) {
		t.Fatalf("无操作后 Team(2) 第 1 层 = %v", got)
	}
}

func TestMoveDeepSubtreeBeyond10Levels(t *testing.T) {
	// 15 层链：环检测必须走完整条上级链，不能只查 10 层。
	records := map[int64]int64{1: 0}
	for i := int64(2); i <= 15; i++ {
		records[i] = i - 1
	}
	tr := New(records)
	if tr.Move(2, 12) {
		t.Fatal("Move(2,12) 在 15 层链中应因成环被拒绝")
	}
	if !tr.Move(15, 1) {
		t.Fatal("Move(15,1) 应成功")
	}
}
