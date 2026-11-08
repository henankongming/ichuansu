// Package netpath refuses to infer a peer address from Xray server endpoints.
// An Xray node address is not automatically the other computer's ichuansu API.
package netpath

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/henankongming/ichuansu/internal/config"
)

var ErrPeerEndpointUnknown = errors.New("æ— æ³•ä»ç°æœ‰ Xray é…ç½®å”¯ä¸€ç¡®å®šè¿œç«¯ ichuansu åº”ç”¨ç«¯ç‚¹")
var ErrListenEndpointUnknown = errors.New("æ— æ³•ä»ç°æœ‰ Xray é…ç½®å”¯ä¸€ç¡®å®šæœ¬æœº ichuansu æœåŠ¡ç›‘å¬åœ°å€")

type LocalPath struct {
	SOCKS config.SOCKSInbound
}

// ResolveLocalSOCKS returns a local Xray SOCKS inbound only when the config
// explicitly declares exactly one usable endpoint. It never assumes default ports.
func ResolveLocalSOCKS(programDir string) (config.SOCKSInbound, error) {
	return config.UniqueSOCKSInbound(filepath.Join(programDir, "i.json"))
}

// ResolvePeerEndpoint intentionally fails closed. Standard Xray server/outbound
// addresses identify Xray infrastructure, not necessarily this user's peer PC.
func ResolvePeerEndpoint(programDir string) (string, error) {
	if _, _, err := config.Load(filepath.Join(programDir, "i.json")); err != nil {
		return "", fmt.Errorf("æœ¬æœº i.json ä¸å¯ç”¨ï¼š%w", err)
	}
	return "", ErrPeerEndpointUnknown
}

// ResolveListenEndpoint intentionally fails closed until the actual interface
// and port are established by a two-host test. Binding to 0.0.0.0 is forbidden.
func ResolveListenEndpoint(programDir string) (string, error) {
	if _, _, err := config.Load(filepath.Join(programDir, "i.json"); err != nil {
		return "", fmt.Errorf("å§+9§.ˆKšœÛÛˆ9.#ycëùå*;ï&‰]È‹\œŠB‚_B‚\™]\›ˆˆ‹\œ“\İ[‘[™Ú[[šÛ›İÛ‚ŸB‚™[˜È^Z[›ØÚÙ\Š
Hİš[™ÈÂ‚\™]\›ˆšKšœÛÛˆ9.+yæ¡˜^H:" ¹à®yg,9g`9.#z ïyæí9£©yodù/g9kîyêëùå-z!$yg,9g`;ï&ùodùbcy.äùn¤ù¬¨y§"ycã9§.ºj£:+àz+¬9oe{ï#9.gù¬¨y§"z+ày£k¹èk¹k¦¹g,9g`:+ëùodù/g9kîyêëùå-z!$yg,9g`8à ˆŠBŸB