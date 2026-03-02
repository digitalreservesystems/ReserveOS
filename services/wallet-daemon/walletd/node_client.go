package walletd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type NodeClient struct {
	BaseURL string
	HC      *http.Client
}

func NewNodeClient(base string) *NodeClient {
	return &NodeClient{BaseURL: base, HC: &http.Client{Timeout: 5 * time.Second}}
}

type ChainInfo struct {
	ChainID string `json:"chain_id"`
	Height  uint64 `json:"height"`
	Tip     string `json:"tip"`
}

func (c *NodeClient) ChainInfo() (*ChainInfo, error) {
	resp, err := c.HC.Get(c.BaseURL + "/chain/info")
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return nil, fmt.Errorf("chain/info %d", resp.StatusCode) }
	var out ChainInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return nil, err }
	return &out, nil
}

type BlockResp struct {
	Height uint64 `json:"height"`
	Hash   string `json:"hash"`
	Block  struct {
		Txs []struct {
			Outputs []struct {
				Amount int64  `json:"amount"`
				Asset  string `json:"asset"`
				Address string `json:"address,omitempty"`
				P string `json:"P,omitempty"`
				R string `json:"R,omitempty"`
				Tag string `json:"tag,omitempty"`
				EncMemo string `json:"enc_memo,omitempty"`
				PolicyBits uint32 `json:"policy_bits,omitempty"`
			} `json:"outputs"`
		} `json:"txs"`
	} `json:"block"`
}

func (c *NodeClient) BlockByHeight(h uint64) (*BlockResp, error) {
	url := c.BaseURL + "/chain/block?height=" + strconv.FormatUint(h, 10)
	resp, err := c.HC.Get(url)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return nil, fmt.Errorf("chain/block %d", resp.StatusCode) }
	var out BlockResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return nil, err }
	return &out, nil
}


func (c *NodeClient) SubmitTx(tx any) (string, error) {
	b, _ := json.Marshal(tx)
	resp, err := c.HC.Post(c.BaseURL+"/tx/submit", "application/json", bytes.NewReader(b))
	if err != nil { return "", err }
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("tx/submit %d", resp.StatusCode)
	}
	if v, ok := out["txid"].(string); ok { return v, nil }
	return "", nil
}


type StateInfo struct {
	ID string `json:"id"`
	Balance int64 `json:"balance"`
	Nonce uint64 `json:"nonce"`
}

func (c *NodeClient) StateInfo(id string) (*StateInfo, error) {
	resp, err := c.HC.Get(c.BaseURL + "/state/info?id=" + id)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return nil, fmt.Errorf("state/info %d", resp.StatusCode) }
	var out StateInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return nil, err }
	return &out, nil
}
