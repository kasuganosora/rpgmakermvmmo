package ai

import (
	"regexp"
	"strconv"
	"strings"
)

// tagRe matches RMMV-style note tags: <key> or <key:value>
var tagRe = regexp.MustCompile(`<([^<>:]+):?([^>]*)>`)

// ParseAIProfile reads <AI:profileName> and optional override tags from an
// Enemy Note field. customProfiles (from MMOConfig) are checked first, then
// DefaultProfiles. Individual fields can be overridden with tags like
// <AI Aggro Range:10>.
//
// Returns nil if no <AI:...> tag is found in the note.
func ParseAIProfile(note string, customProfiles map[string]*AIProfile) *AIProfile {
	if note == "" {
		return nil
	}
	matches := tagRe.FindAllStringSubmatch(note, -1)
	if len(matches) == 0 {
		return nil
	}

	var base *AIProfile
	for _, m := range matches {
		key := strings.TrimSpace(m[1])
		val := strings.TrimSpace(m[2])
		if strings.EqualFold(key, "AI") && val != "" {
			profileName := strings.ToLower(val)
			if customProfiles != nil {
				if p, ok := customProfiles[profileName]; ok {
					cp := *p
					base = &cp
					break
				}
			}
			if p, ok := DefaultProfiles[profileName]; ok {
				cp := *p
				base = &cp
				break
			}
		}
	}

	if base == nil {
		return nil
	}

	// Apply override tags.
	for _, m := range matches {
		key := strings.ToLower(strings.TrimSpace(m[1]))
		val := strings.TrimSpace(m[2])
		if val == "" {
			continue
		}
		n, err := strconv.Atoi(val)
		if err != nil {
			continue
		}
		switch key {
		case "ai aggro range":
			base.AggroRange = n
		case "ai leash range":
			base.LeashRange = n
		case "ai attack range":
			base.AttackRange = n
		case "ai attack cooldown":
			base.AttackCooldownTicks = n
		case "ai move interval":
			base.MoveIntervalTicks = n
		case "ai wander radius":
			base.WanderRadius = n
		case "ai flee hp":
			base.FleeHPPercent = n
		}
	}

	return base
}
