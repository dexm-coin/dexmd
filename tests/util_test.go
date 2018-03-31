package tests

import (
	"math"
	"testing"

	"github.com/dexm-coin/dexmd/util"
)

func TestOverflow(t *testing.T) {
	res, ok := util.AddU32O(1, 1)

	if res != 2 || !ok {
		t.Error("1 + 1 != 2 or overflows. Got ", res, ok)
	}

	_, ok = util.AddU32O(math.MaxUint32-100, 500)
	if ok {
		t.Error("AddU32O didn't catch overflow")
	}

	res, ok = util.SubU32O(2, 1)
	if res != 1 || !ok {
		t.Error("2-1 != 1 or overflows. Got ", res, ok)
	}

	res, ok = util.SubU32O(100, 200)
	if ok {
		t.Error("AddU32O didn't catch overflow", res)
	}
}
