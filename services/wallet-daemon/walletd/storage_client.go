package walletd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type StorageClient struct {
	BaseURL string
	HC      *http.Client
}

func NewStorageClient(base string) *StorageClient {
	return &StorageClient{BaseURL: base, HC: &http.Client{Timeout: 5 * time.Second}}
}

type AllocSlotReq struct {
	AccountID *string `json:"account_id"`
	Purpose   string  `json:"purpose"`
	ExpiresAt *int64  `json:"expires_at"`
}

type AllocSlotResp struct {
	SlotID          int64  `json:"slot_id"`
	DerivationIndex uint64 `json:"derivation_index"`
	ExpiresAt       *int64 `json:"expires_at"`
}

func (c *StorageClient) AllocSlot(req AllocSlotReq) (*AllocSlotResp, error) {
	b, _ := json.Marshal(req)
	resp, err := c.HC.Post(c.BaseURL+"/alloc_slot", "application/json", bytes.NewReader(b))
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return nil, fmt.Errorf("alloc_slot status %d", resp.StatusCode) }
	var out AllocSlotResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return nil, err }
	return &out, nil
}

func (c *StorageClient) MarkConfirmed(slotID int64, height int64, txid string, confirmedAt int64) error {
	body := map[string]any{"slot_id": slotID, "height": height, "txid": txid, "confirmed_at": confirmedAt}
	b, _ := json.Marshal(body)
	resp, err := c.HC.Post(c.BaseURL+"/mark_confirmed", "application/json", bytes.NewReader(b))
	if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return fmt.Errorf("mark_confirmed status %d", resp.StatusCode) }
	return nil
}
