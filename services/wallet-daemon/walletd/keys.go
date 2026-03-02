package walletd

import (
	"encoding/json"
	"errors"
	"os"

	"reserveos/core/crypto/otap"
	"reserveos/internal/reservekeyvault"
)

// Keyvault keys
const (
	kvOTAPSPriv = "otap.s_priv"
	kvOTAPVPriv = "otap.v_priv"
	kvOTAPSPub  = "otap.S_pub"
	kvOTAPVPub  = "otap.V_pub"
)

type OTAPKeys struct {
	Registry otap.RegistryKeys
	Priv     otap.PrivateKeys
}

// LoadOrGenOTAPKeys loads OTAP keys from keyvault; generates and persists if missing.
func LoadOrGenOTAPKeys(v *reservekeyvault.Vault) (*OTAPKeys, error) {
	if v == nil {
		return nil, errors.New("nil keyvault")
	}

	sPriv, okS := v.GetString(kvOTAPSPriv)
	vPriv, okV := v.GetString(kvOTAPVPriv)
	sPub, okSP := v.GetString(kvOTAPSPub)
	vPub, okVP := v.GetString(kvOTAPVPub)

	if okS && okV && okSP && okVP {
		return &OTAPKeys{
			Registry: otap.RegistryKeys{ScanPub: sPub, SpendPub: vPub},
			Priv:     otap.PrivateKeys{ScanPriv: sPriv, SpendPriv: vPriv},
		}, nil
	}

	reg, priv, err := otap.GenerateKeys()
	if err != nil {
		return nil, err
	}
	// Persist
	if err := v.SetString(kvOTAPSPriv, priv.ScanPriv); err != nil { return nil, err }
	if err := v.SetString(kvOTAPVPriv, priv.SpendPriv); err != nil { return nil, err }
	if err := v.SetString(kvOTAPSPub, reg.ScanPub); err != nil { return nil, err }
	if err := v.SetString(kvOTAPVPub, reg.SpendPub); err != nil { return nil, err }

	b, _ := json.MarshalIndent(map[string]any{
		"S_pub": reg.ScanPub,
		"V_pub": reg.SpendPub,
		"note": "Persisted to keyvault.",
	}, "", "  ")
	_, _ = os.Stderr.Write([]byte("\n[wallet-daemon] Generated OTAP keys and stored in keyvault:\n" + string(b) + "\n\n"))

	return &OTAPKeys{Registry: reg, Priv: priv}, nil
}
