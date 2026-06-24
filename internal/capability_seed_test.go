package internal

import (
	"strings"
	"testing"
)

// TestCapabilitySeedMetadata enforces the SSOT invariants behind tier-0 discovery:
// every op is categorized, novelty is consistent with featured categories, and the
// rendered seed names every featured category. A new op or novel flag that breaks
// these fails here rather than silently shipping an op the browse/seed can't reach.
func TestCapabilitySeedMetadata(t *testing.T) {
	supported := make(map[string]bool, len(pluginSupportedBatchOps))
	for _, op := range pluginSupportedBatchOps {
		supported[op] = true
	}

	// 1. Every plugin-supported op has an intent category.
	for _, op := range pluginSupportedBatchOps {
		if _, ok := batchOpIntent[op]; !ok {
			t.Errorf("op %q is missing from batchOpIntent — browse/seed cannot reach it", op)
		}
	}

	// 2. No stray intent entries for ops that no longer exist.
	for op := range batchOpIntent {
		if !supported[op] {
			t.Errorf("batchOpIntent has %q which is not in pluginSupportedBatchOps", op)
		}
	}

	// 3. Every novel op exists and sits in a FEATURED category (one with a teaser).
	//    This is the data-driven link: novelty makes a category featured.
	for op := range novelBatchOps {
		if !supported[op] {
			t.Errorf("novelBatchOps has %q which is not a plugin-supported op", op)
			continue
		}
		cat := batchOpIntent[op]
		if _, ok := categoryTeaser[cat]; !ok {
			t.Errorf("novel op %q is in category %q which has no categoryTeaser; a category with a novel op must be featured", op, cat)
		}
		if _, ok := categoryIntentLabel[cat]; !ok {
			t.Errorf("novel op %q is in category %q which has no categoryIntentLabel", op, cat)
		}
	}

	// 4. Featured order, labels, and teasers are consistent.
	for _, cat := range featuredCategoryOrder {
		if _, ok := categoryIntentLabel[cat]; !ok {
			t.Errorf("featured category %q has no categoryIntentLabel", cat)
		}
		if _, ok := categoryTeaser[cat]; !ok {
			t.Errorf("featured category %q has no categoryTeaser", cat)
		}
	}
	if len(categoryTeaser) != len(featuredCategoryOrder) {
		t.Errorf("categoryTeaser has %d entries but featuredCategoryOrder has %d — keep them in sync", len(categoryTeaser), len(featuredCategoryOrder))
	}

	// 5. The rendered seed names every featured category and routes the flow.
	seed := BuildCapabilitySeed()
	for _, cat := range featuredCategoryOrder {
		if !strings.Contains(seed, "category:"+cat) {
			t.Errorf("capability seed is missing featured category %q", cat)
		}
	}
	for _, want := range []string{"search_batch_ops(category)", "get_batch_op_spec(op)", "figma-design-patterns"} {
		if !strings.Contains(seed, want) {
			t.Errorf("capability seed must route the discovery flow; missing %q", want)
		}
	}
}
