package battle

import (
	"math"
	"testing"
)

func TestEvalFormula_BasicArithmetic(t *testing.T) {
	a := &CharacterStats{Atk: 30, Def: 20, Mat: 25, Mdf: 15}
	b := &CharacterStats{Atk: 10, Def: 10, Mat: 8, Mdf: 6}

	v, err := EvalFormula("a.atk * 4 - b.def * 2", a, b)
	if err != nil {
		t.Fatal(err)
	}
	if v != 100 {
		t.Errorf("got %f, want 100", v)
	}
}

func TestEvalFormula_MathFloor(t *testing.T) {
	a := &CharacterStats{Atk: 10}
	b := &CharacterStats{Def: 3}

	v, err := EvalFormula("Math.floor(a.atk / b.def)", a, b)
	if err != nil {
		t.Fatal(err)
	}
	if v != 3 {
		t.Errorf("got %f, want 3", v)
	}
}

// --- Custom formula function tests (ProjectB Damagecul.js) ---

func TestEvalFormula_DamageCulNormal(t *testing.T) {
	// damagecul_normal(a_atk, b_def, level, e_enhance)
	// = (a_atk - b_def/2) * 2 * level * ((enhance+100)/100)
	// Server ignores $gameVariables enhancement → enhance=0 → multiplier=1.0
	a := &CharacterStats{Atk: 30}
	b := &CharacterStats{Def: 10}

	// damagecul_normal(a.atk, b.def, 3, 0)
	// = (30 - 10/2) * 2 * 3 = 25 * 6 = 150
	v, err := EvalFormula("damagecul_normal(a.atk, b.def, 3, 0)", a, b)
	if err != nil {
		t.Fatal(err)
	}
	if v != 150 {
		t.Errorf("got %f, want 150", v)
	}
}

func TestEvalFormula_DamageCulMagic(t *testing.T) {
	// damagecul_magic(a.mat, b.mdf, 3, 1224)
	// = (a.mat - b.mdf/2) * 2 * 3 (enhancement from game vars ignored)
	a := &CharacterStats{Mat: 25}
	b := &CharacterStats{Mdf: 10}

	// = (25 - 10/2) * 2 * 3 = 20 * 6 = 120
	v, err := EvalFormula("damagecul_magic(a.mat, b.mdf, 3, 1224)", a, b)
	if err != nil {
		t.Fatal(err)
	}
	if v != 120 {
		t.Errorf("got %f, want 120", v)
	}
}

func TestEvalFormula_DamageCulEnemyNormal(t *testing.T) {
	// damagecul_enemy_normal(a_atk, b_def, level)
	// = (a_atk - b_def/2) * 2 * level
	a := &CharacterStats{Atk: 14}
	b := &CharacterStats{Def: 10}

	// = (14 - 10/2) * 2 * 2 = 9 * 4 = 36
	v, err := EvalFormula("damagecul_enemy_normal(a.atk, b.def, 2)", a, b)
	if err != nil {
		t.Fatal(err)
	}
	if v != 36 {
		t.Errorf("got %f, want 36", v)
	}
}

func TestEvalFormula_DamageCulPenetration(t *testing.T) {
	// damagecul_penetration(a_atk, b_def, level)
	// = (a_atk - b_def/4) * 2 * level
	a := &CharacterStats{Atk: 20}
	b := &CharacterStats{Def: 12}

	// = (20 - 12/4) * 2 * 3 = 17 * 6 = 102
	v, err := EvalFormula("damagecul_penetration(a.atk, b.def, 3)", a, b)
	if err != nil {
		t.Fatal(err)
	}
	if v != 102 {
		t.Errorf("got %f, want 102", v)
	}
}

func TestEvalFormula_DamageCulMat01(t *testing.T) {
	// damagecul_mat01(a_mat, b_mdf, level)
	// = (a_mat - b_mdf/2) * 2 * level
	a := &CharacterStats{Mat: 30}
	b := &CharacterStats{Mdf: 10}

	// = (30 - 10/2) * 2 * 2 = 25 * 4 = 100
	v, err := EvalFormula("damagecul_mat01(a.mat, b.mdf, 2)", a, b)
	if err != nil {
		t.Fatal(err)
	}
	if v != 100 {
		t.Errorf("got %f, want 100", v)
	}
}

func TestEvalFormula_DamageCulEnemyNormalMinDamage(t *testing.T) {
	// When damage formula evaluates to <= 0, damagecul_enemy_normal returns random 0-2
	a := &CharacterStats{Atk: 1}
	b := &CharacterStats{Def: 100}

	// = (1 - 100/2) * 2 * 2 = -49 * 4 = -196 → clamp to random 0-2
	v, err := EvalFormula("damagecul_enemy_normal(a.atk, b.def, 2)", a, b)
	if err != nil {
		t.Fatal(err)
	}
	// Should be 0, 1, or 2
	if v < 0 || v > 2 {
		t.Errorf("got %f, want 0-2 (min damage fallback)", v)
	}
}

func TestEvalFormula_ProjectBSkill71(t *testing.T) {
	// Skill 71 (光之矢): damagecul_magic(a.mat, b.mdf, 3, 1224)
	// Actor: class 1 level 1, mat from base params
	// Class 1 base params at level 1: mat=10 (index 4)
	// Enemy 36: mdf=10
	a := &CharacterStats{Mat: 10}
	b := &CharacterStats{Mdf: 10}

	v, err := EvalFormula("damagecul_magic(a.mat, b.mdf, 3, 1224)", a, b)
	if err != nil {
		t.Fatal(err)
	}
	// = (10 - 10/2) * 2 * 3 = 5 * 6 = 30
	if v != 30 {
		t.Errorf("got %f, want 30", v)
	}
}

func TestEvalFormula_ProjectBSkill263(t *testing.T) {
	// Skill 263 (enemy attack): damagecul_enemy_normal(a.atk, b.def, 2)
	// Enemy 36: atk=14
	// Actor: def=10
	a := &CharacterStats{Atk: 14}
	b := &CharacterStats{Def: 10}

	v, err := EvalFormula("damagecul_enemy_normal(a.atk, b.def, 2)", a, b)
	if err != nil {
		t.Fatal(err)
	}
	// = (14 - 10/2) * 2 * 2 = 9 * 4 = 36
	if v != 36 {
		t.Errorf("got %f, want 36", v)
	}
}

func TestEvalFormula_UnknownFuncReturnsError(t *testing.T) {
	a := &CharacterStats{Atk: 10}
	b := &CharacterStats{Def: 5}

	_, err := EvalFormula("totally_unknown_func(a.atk, b.def)", a, b)
	if err == nil {
		t.Error("expected error for unknown function")
	}
}

func TestEvalFormula_NestedExpressionsInCustomFunc(t *testing.T) {
	// Custom function args can be complex expressions
	a := &CharacterStats{Atk: 10, Mat: 20}
	b := &CharacterStats{Def: 5, Mdf: 10}

	// damagecul_normal(a.atk + a.mat, b.def + b.mdf, 2, 0)
	// = ((10+20) - (5+10)/2) * 2 * 2 = (30 - 7.5) * 4 = 22.5 * 4 = 90
	v, err := EvalFormula("damagecul_normal(a.atk + a.mat, b.def + b.mdf, 2, 0)", a, b)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(v-90) > 0.01 {
		t.Errorf("got %f, want 90", v)
	}
}
