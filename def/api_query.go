package def

import "fmt"

// apiPredicate is an XPath predicate that is true for a registry node that applies to the named API: it has no api
// attribute, or the attribute is a comma-separated list that includes the API (for example api="vulkan,vulkansc").
func apiPredicate(api string) string {
	return fmt.Sprintf("(contains(concat(',', @api, ','), ',%s,') or not(@api))", api)
}
