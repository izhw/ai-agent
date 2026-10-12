package portable

import (
	"context"
	"testing"

	memoryadapter "github.com/izhw/ai-agent/adapter/storage/memory"
	"github.com/izhw/ai-agent/contract"
	memoryspi "github.com/izhw/ai-agent/spi/memory"
)

func TestExplicitMemoryAndRecall(t *testing.T) {
	ctx := context.Background()
	store := memoryadapter.New()
	service := NewService(store.Memories(), nil)
	run := contract.Run{ID: "run-1", TenantID: "t", ProductID: "p", UserID: "u", AgentKey: "a"}
	err := service.Remember(
		ctx,
		memoryspi.ExtractInput{
			Run:      run,
			Policy:   contract.MemoryPolicy{Mode: contract.MemoryReadWrite, RequireExplicit: true},
			Messages: []contract.Message{{Role: contract.RoleUser, Content: "记住：我的数据库是达梦"}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.Recall(
		ctx,
		memoryspi.Query{
			TenantID:  "t",
			ProductID: "p",
			UserID:    "u",
			AgentKey:  "a",
			Text:      "数据库",
			Limit:     3,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Content != "我的数据库是达梦" {
		t.Fatalf("unexpected result %#v", items)
	}
}
