package riskcontrol

import (
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestKeywordEngine_BasicMatch(t *testing.T) {
	engine := NewKeywordEngine(domain.KeywordConfig{
		Enabled: true,
		Entries: []domain.KeywordEntry{
			{Word: "badword", Level: domain.KeywordLevelBlock},
		},
	})
	match := engine.Match("this has badword in it")
	if match == nil || match.Entry.Word != "badword" {
		t.Fatalf("expected match 'badword', got %#v", match)
	}
	if match.HitLayer != "keyword" {
		t.Fatalf("expected hit_layer=keyword, got %s", match.HitLayer)
	}
}

func TestKeywordEngine_CaseInsensitive(t *testing.T) {
	engine := NewKeywordEngine(domain.KeywordConfig{
		Enabled: true,
		Entries: []domain.KeywordEntry{
			{Word: "BadWord", Level: domain.KeywordLevelBlock},
		},
	})
	match := engine.Match("this has BADWORD in it")
	if match == nil || match.Entry.Word != "BadWord" {
		t.Fatalf("expected match 'BadWord', got %#v", match)
	}
}

func TestKeywordEngine_FullWidth(t *testing.T) {
	engine := NewKeywordEngine(domain.KeywordConfig{
		Enabled: true,
		Entries: []domain.KeywordEntry{
			{Word: "badword", Level: domain.KeywordLevelBlock},
		},
	})
	// Full-width letters should normalize and match.
	match := engine.Match("ｂａｄｗｏｒｄ")
	if match == nil {
		t.Fatal("expected match with full-width input")
	}
}

func TestKeywordEngine_TraditionalChinese(t *testing.T) {
	engine := NewKeywordEngine(domain.KeywordConfig{
		Enabled: true,
		Entries: []domain.KeywordEntry{
			{Word: "敏感词", Level: domain.KeywordLevelBlock},
		},
	})
	// Traditional characters should convert to simplified and match.
	match := engine.Match("這是敏感詞")
	if match == nil || match.Entry.Word != "敏感词" {
		t.Fatalf("expected match '敏感词', got %#v", match)
	}
}

func TestKeywordEngine_SeparatorBypass(t *testing.T) {
	engine := NewKeywordEngine(domain.KeywordConfig{
		Enabled: true,
		Entries: []domain.KeywordEntry{
			{Word: "敏感词", Level: domain.KeywordLevelBlock},
		},
	})
	// "敏*感*词" should match via the stripped view.
	match := engine.Match("这是敏*感*词测试")
	if match == nil {
		t.Fatal("expected match with separator bypass")
	}
}

func TestKeywordEngine_RequireWith(t *testing.T) {
	engine := NewKeywordEngine(domain.KeywordConfig{
		Enabled: true,
		Entries: []domain.KeywordEntry{
			{
				Word:        "枪",
				Level:       domain.KeywordLevelBlock,
				RequireWith: []string{"买"},
			},
		},
	})
	// "枪" alone should NOT match (require_with "买" not present).
	match := engine.Match("枪版电影")
	if match != nil {
		t.Fatalf("expected no match without co-occurrence, got %#v", match)
	}
	// "买枪" should match.
	match = engine.Match("我要买枪")
	if match == nil || match.Entry.Word != "枪" {
		t.Fatalf("expected match with co-occurrence, got %#v", match)
	}
}

func TestKeywordEngine_SuspectLevel(t *testing.T) {
	engine := NewKeywordEngine(domain.KeywordConfig{
		Enabled: true,
		Entries: []domain.KeywordEntry{
			{Word: "suspectword", Level: domain.KeywordLevelSuspect},
		},
	})
	match := engine.Match("this has suspectword in it")
	if match == nil {
		t.Fatal("expected match")
	}
	if match.Entry.Level != domain.KeywordLevelSuspect {
		t.Fatalf("expected suspect level, got %s", match.Entry.Level)
	}
}

func TestKeywordEngine_BlockTakesPriorityOverSuspect(t *testing.T) {
	engine := NewKeywordEngine(domain.KeywordConfig{
		Enabled: true,
		Entries: []domain.KeywordEntry{
			{Word: "suspectword", Level: domain.KeywordLevelSuspect},
			{Word: "blockword", Level: domain.KeywordLevelBlock},
		},
	})
	match := engine.Match("has suspectword and blockword")
	if match == nil || match.Entry.Level != domain.KeywordLevelBlock {
		t.Fatalf("expected block priority, got %#v", match)
	}
}

func TestKeywordEngine_NoMatch(t *testing.T) {
	engine := NewKeywordEngine(domain.KeywordConfig{
		Enabled: true,
		Entries: []domain.KeywordEntry{
			{Word: "badword", Level: domain.KeywordLevelBlock},
		},
	})
	match := engine.Match("clean text")
	if match != nil {
		t.Fatalf("expected no match, got %#v", match)
	}
}

func TestKeywordEngine_EmptyEngine(t *testing.T) {
	engine := NewKeywordEngine(domain.KeywordConfig{Enabled: true})
	match := engine.Match("anything")
	if match != nil {
		t.Fatalf("expected nil match from empty engine, got %#v", match)
	}
}

func TestKeywordEngine_NilEngine(t *testing.T) {
	var engine *KeywordEngine
	match := engine.Match("anything")
	if match != nil {
		t.Fatalf("expected nil match from nil engine, got %#v", match)
	}
}

func TestKeywordEngine_PinyinMatch(t *testing.T) {
	engine := NewKeywordEngine(domain.KeywordConfig{
		Enabled: true,
		Entries: []domain.KeywordEntry{
			{Word: "test", Level: domain.KeywordLevelBlock},
		},
		Pinyin: domain.PinyinConfig{
			Enabled: true,
			Entries: []domain.KeywordEntry{
				{Word: "微信", Level: domain.KeywordLevelBlock},
			},
		},
	})
	// "葳信" → pinyin "weixin" should match "微信" → pinyin "weixin".
	match := engine.Match("加我葳信")
	if match == nil {
		t.Fatal("expected pinyin match for '葳信'")
	}
	if match.HitLayer != "pinyin" {
		t.Fatalf("expected hit_layer=pinyin, got %s", match.HitLayer)
	}
	if match.Entry.Word != "微信" {
		t.Fatalf("expected matched word '微信', got %s", match.Entry.Word)
	}
}

func TestKeywordEngine_PinyinDisabled(t *testing.T) {
	engine := NewKeywordEngine(domain.KeywordConfig{
		Enabled: true,
		Entries: []domain.KeywordEntry{
			{Word: "test", Level: domain.KeywordLevelBlock},
		},
		Pinyin: domain.PinyinConfig{
			Enabled: false,
			Entries: []domain.KeywordEntry{
				{Word: "微信", Level: domain.KeywordLevelBlock},
			},
		},
	})
	// Pinyin disabled, "葳信" should NOT match.
	match := engine.Match("加我葳信")
	if match != nil {
		t.Fatalf("expected no pinyin match when disabled, got %#v", match)
	}
}

func TestKeywordEngine_KeywordTakesPriorityOverPinyin(t *testing.T) {
	engine := NewKeywordEngine(domain.KeywordConfig{
		Enabled: true,
		Entries: []domain.KeywordEntry{
			{Word: "葳信", Level: domain.KeywordLevelBlock},
		},
		Pinyin: domain.PinyinConfig{
			Enabled: true,
			Entries: []domain.KeywordEntry{
				{Word: "微信", Level: domain.KeywordLevelBlock},
			},
		},
	})
	// "葳信" should match the keyword entry, not the pinyin entry.
	match := engine.Match("加我葳信")
	if match == nil {
		t.Fatal("expected match")
	}
	if match.HitLayer != "keyword" {
		t.Fatalf("expected keyword layer (priority), got %s", match.HitLayer)
	}
}
