package service

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"

	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/userdata"
)

const termsVersionMarker = "###123###"
const privacyVersionMarker = "###123###"

// UserSettings holds and transmits the user's settings.
// AutoComplete mirrors the "Auto-complete hidden story unlock missions"
// checkbox and maps directly onto the DB bit: checked/1 presents those
// missions as already completed; unchecked/0 (the default) the player must
// clear them to unlock the hidden stories.
// ShowNews mirrors the "Show in-game news popups" checkbox: checked/1 (the
// default for every player) shows news normally; unchecked/0 reports active
// dokans as already-seen in the projection without any DB write.
// ForceQuestMissions mirrors the "Force quest missions" checkbox: checked/1
// makes the three additional challenge missions inside the quest currently
// being cleared (plus their "Clear all missions" meta-mission) count as
// achieved, recorded in the database with their rewards; unchecked/0 (the
// default) requires clearing them legitimately.
type UserSettings struct {
	MasterdataVariant  string `json:"masterdata_variant"`
	AutoComplete       bool   `json:"auto_complete"`
	ForceQuestMissions bool   `json:"force_quest_missions"`
	ShowNews           bool   `json:"show_news"`
}

// displayAllVariant is the web page's label for the base master-data file; it
// maps to/from the empty DB token (no "_Variant" suffix in the version string).
const displayAllVariant = "All"

// resolveUserID maps the identifier the web page carries (numeric user_id or
// account uuid, see extractUserID) onto a users.user_id. Unidentified requests
// fall back to the oldest account so a locally opened page stays usable.
func (s *OctoHTTPServer) resolveUserID(identifier string) (int64, error) {
	if id, err := strconv.ParseInt(identifier, 10, 64); err == nil && id > 0 {
		return id, nil
	}
	if id, err := s.users.GetUserByUUID(identifier); err == nil {
		return id, nil
	}
	return s.users.DefaultUserId()
}

// getUserSettings loads a user's options from the user_options table. The
// auto-complete and force-quest-missions checkboxes default to unchecked
// (missions must be played) and the news checkbox defaults to checked for
// users who never saved.
func (s *OctoHTTPServer) getUserSettings(identifier string) UserSettings {
	settings := UserSettings{MasterdataVariant: displayAllVariant, AutoComplete: false, ForceQuestMissions: false, ShowNews: true}
	userId, err := s.resolveUserID(identifier)
	if err != nil {
		log.Printf("[DB] no user for identifier %q: %v; serving defaults", identifier, err)
		return settings
	}
	opts, err := s.users.GetUserOptions(userId)
	if err != nil {
		log.Printf("[DB] load options for user %d: %v", userId, err)
		return settings
	}
	if opts.MasterDataVariant != "" {
		settings.MasterdataVariant = opts.MasterDataVariant
	}
	settings.AutoComplete = opts.ForceHiddenStoryMissions
	settings.ShowNews = opts.ShowNews
	settings.ForceQuestMissions = opts.ForceQuestMissions
	return settings
}

// saveUserSettings persists a user's options to the user_options table.
func (s *OctoHTTPServer) saveUserSettings(identifier string, settings UserSettings) error {
	userId, err := s.resolveUserID(identifier)
	if err != nil {
		return fmt.Errorf("no user for identifier %q: %w", identifier, err)
	}
	opts := store.UserOptionsState{
		MasterDataVariant:        settings.MasterdataVariant,
		ForceHiddenStoryMissions: settings.AutoComplete,
		ShowNews:                 settings.ShowNews,
		ForceQuestMissions:       settings.ForceQuestMissions,
	}
	if opts.MasterDataVariant == displayAllVariant {
		opts.MasterDataVariant = ""
	}
	if err := s.users.SaveUserOptions(userId, opts); err != nil {
		return fmt.Errorf("save options for user %d: %w", userId, err)
	}
	log.Printf("[DB] Saved settings for userID: %s -> Variant: %q, AutoCompleteHiddenMissions: %v, ShowNews: %v, ForceQuestMissions: %v", identifier, opts.MasterDataVariant, opts.ForceHiddenStoryMissions, opts.ShowNews, opts.ForceQuestMissions)
	return nil
}

// extractUserID retrieves the user ID from URL query parameters, HTTP headers, or cookies
func extractUserID(r *http.Request) string {
	if userID := r.URL.Query().Get("userId"); userID != "" {
		return userID
	}
	if playerID := r.URL.Query().Get("playerId"); playerID != "" {
		return playerID
	}
	if userID := r.Header.Get("X-User-Id"); userID != "" {
		return userID
	}
	if userID := r.Header.Get("X-Session-Id"); userID != "" {
		return userID
	}
	if cookie, err := r.Cookie("user_id"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return "default_user"
}

// getMasterDataVariants scans the assets/release folder and collects the list of all variants
func (s *OctoHTTPServer) getMasterDataVariants() []string {
	releaseDir := filepath.Join(s.BaseDir, "assets", "release")
	files, err := os.ReadDir(releaseDir)
	if err != nil {
		log.Printf("[MasterData] Error reading release directory: %v", err)
		return []string{"All"}
	}

	const basePrefix = "20240404193219"
	const extension = ".bin.e"

	variantsMap := make(map[string]bool)

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		name := file.Name()
		if strings.HasPrefix(name, basePrefix) && strings.HasSuffix(name, extension) {
			middle := strings.TrimPrefix(name, basePrefix)
			middle = strings.TrimSuffix(middle, extension)

			if middle == "" {
				variantsMap["All"] = true
			} else if strings.HasPrefix(middle, "_") {
				variant := strings.TrimPrefix(middle, "_")
				if variant != "" {
					variantsMap[variant] = true
				}
			}
		}
	}

	var variants []string
	if variantsMap["All"] {
		variants = append(variants, "All")
		delete(variantsMap, "All")
	}

	for v := range variantsMap {
		variants = append(variants, v)
	}

	if len(variants) == 0 {
		return []string{"All"}
	}

	return variants
}

func renderInformationPage(variants []string, currentVariant string, autoComplete bool, forceQuestMissions bool, showNews bool) string {
	var variantsJS []string
	for _, v := range variants {
		variantsJS = append(variantsJS, fmt.Sprintf("%q", v))
	}
	jsArray := "[" + strings.Join(variantsJS, ", ") + "]"

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Lunar Tear</title>
<style>
  body { margin:0; padding:40px 20px; font-family:"Noto Sans",sans-serif;
         background:#0a0a0f; color:#d4cfc6; text-align:center; }
  h1   { font-size:1.4em; letter-spacing:.15em; color:#e8e0d0; margin-bottom:4px; }
  .sub { font-size:.75em; color:#888; margin-bottom:32px; }
  .sep { width:60px; border:none; border-top:1px solid #333; margin:24px auto; }
  p    { font-size:.85em; line-height:1.6; color:#999; max-width:360px; margin:0 auto 12px; }
  
  .section-title {
    font-size: 0.9em;
    letter-spacing: .1em;
    color: #e8e0d0;
    margin-top: 16px;
    margin-bottom: 16px;
  }

  .field-label {
    font-size: 0.8em;
    color: #888;
    margin-bottom: 8px;
  }

  .variant-picker {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    margin-bottom: 20px;
  }
  .variant-btn {
    background: #1a1a24;
    color: #e8e0d0;
    border: 1px solid #333;
    padding: 6px 14px;
    font-size: 0.9em;
    cursor: pointer;
    border-radius: 4px;
    transition: background 0.2s, opacity 0.2s;
  }
  .variant-btn:hover:not(:disabled) {
    background: #2a2a38;
  }
  .variant-btn:disabled {
    opacity: 0.3;
    cursor: not-allowed;
  }
  .variant-input {
    background: #12121a;
    color: #a0c4e8;
    border: 1px solid #333;
    padding: 6px 10px;
    font-size: 0.85em;
    text-align: center;
    width: 120px;
    border-radius: 4px;
    outline: none;
    font-family: monospace;
  }

  .option-toggle {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    font-size: 0.85em;
    color: #999;
    cursor: pointer;
    user-select: none;
  }
  .option-toggle input[type="checkbox"] {
    cursor: pointer;
    accent-color: #a0c4e8;
  }

  .reload-note {
    color: #e05555;
    font-size: 0.72em;
    letter-spacing: .03em;
    margin: -8px 0 18px;
  }
</style>
</head>
<body>
  <h1>LUNAR TEAR</h1>
  <div class="sub">Private Preservation Server</div>
  <hr class="sep">
  <p>A community effort to keep NieR Re[in]carnation playable after official service ended.</p>
  <p>This server is not affiliated with or endorsed by SQUARE ENIX or Applibot.</p>
  <hr class="sep">
  <p style="font-size:.7em;color:#666;">&copy; SQUARE ENIX / Applibot — All game assets belong to their respective owners.</p>
  <hr class="sep">
  
  <div class="section-title">OPTIONS</div>
  <div class="reload-note">After changing settings you need to reload the client</div>

  <div class="field-label">Masterdata variant</div>
  <div class="variant-picker">
    <button id="btnPrev" class="variant-btn" onclick="changeVariant(-1)">&lsaquo;</button>
    <input id="variantDisplay" class="variant-input" type="text" readonly value="" />
    <button id="btnNext" class="variant-btn" onclick="changeVariant(1)">&rsaquo;</button>
  </div>

  <label class="option-toggle">
    <span>Auto-complete hidden story unlock missions</span>
    <input type="checkbox" id="hiddenMissionsCheckbox" onchange="saveSettings()">
  </label>

  <label class="option-toggle">
    <span>Force quest missions</span>
    <input type="checkbox" id="forceQuestMissionsCheckbox" onchange="saveSettings()">
  </label>

  <label class="option-toggle">
    <span>Show in-game news popups</span>
    <input type="checkbox" id="showNewsCheckbox" onchange="saveSettings()">
  </label>
  
   <hr class="sep">
	  <div class="section-title">ADMIN</div>
	  <div class="field-label">Send item to mailbox (userId possessionType possessionId count)</div>
	  <div style="margin-bottom: 12px;">
		<input id="adminGiftInput" class="variant-input" type="text" placeholder="1 5 313016 5"
			   style="width: 260px; text-align: left;" />
	  </div>
	  <button id="adminGiftBtn" class="variant-btn" onclick="sendAdminGift()">Send to mail</button>
	  <div id="adminGiftStatus" style="font-size:0.75em;color:#888;margin-top:10px;"></div>

<script>
  const variants = %s;
  const initialVariant = %q;
  const initialAutoComplete = %t;
  const initialForceQuestMissions = %t;
  const initialShowNews = %t;

  let currentIndex = 0;

  function initUI() {
    // 1. Find the variant from the database among the scanned files
    let targetIndex = variants.indexOf(initialVariant);
    if (targetIndex === -1) {
      // If this variant is not among the files, default to "All"
      targetIndex = variants.indexOf("All");
      if (targetIndex === -1) targetIndex = 0;
    }
    currentIndex = targetIndex;

    // 2. Set the checkboxes from the database
    document.getElementById('hiddenMissionsCheckbox').checked = initialAutoComplete;
    document.getElementById('forceQuestMissionsCheckbox').checked = initialForceQuestMissions;
    document.getElementById('showNewsCheckbox').checked = initialShowNews;

    updateUI();
  }

  function updateUI() {
    const input = document.getElementById('variantDisplay');
    const btnPrev = document.getElementById('btnPrev');
    const btnNext = document.getElementById('btnNext');

    if (variants.length > 0) {
      input.value = variants[currentIndex];
    } else {
      input.value = "None";
    }

    const isDisabled = variants.length <= 1;
    btnPrev.disabled = isDisabled;
    btnNext.disabled = isDisabled;
  }

  function changeVariant(dir) {
    if (variants.length <= 1) return;
    currentIndex = (currentIndex + dir + variants.length) %% variants.length;
    updateUI();
    saveSettings();
  }

  function saveSettings() {
    const urlParams = new URLSearchParams(window.location.search);
    const userId = urlParams.get('userId') || 'default_user';

    const data = JSON.stringify({
      masterdata_variant: variants[currentIndex] || "All",
      auto_complete: document.getElementById('hiddenMissionsCheckbox').checked,
      force_quest_missions: document.getElementById('forceQuestMissionsCheckbox').checked,
      show_news: document.getElementById('showNewsCheckbox').checked
    });

    const targetUrl = '/information/save?userId=' + encodeURIComponent(userId);

    if (navigator.sendBeacon) {
      const blob = new Blob([data], { type: 'application/json' });
      navigator.sendBeacon(targetUrl, blob);
    } else if (typeof fetch === 'function') {
      fetch(targetUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: data,
        keepalive: true
      });
    }
  }
  
   function sendAdminGift() {
    const input = document.getElementById('adminGiftInput');
    const status = document.getElementById('adminGiftStatus');
    const raw = (input.value || '').trim();
    if (!raw) {
      status.textContent = 'Empty input';
      status.className = 'err';
      return;
    }
    status.textContent = 'Sending...';
    status.className = '';

    fetch('/information/admin/gift', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ line: raw })
    })
    .then(r => r.json().then(j => ({ ok: r.ok, j })))
    .then(({ ok, j }) => {
      if (ok) {
        status.textContent = j.message || 'OK';
        status.className = 'ok';
        input.value = '';
      } else {
        status.textContent = j.error || 'Failed';
        status.className = 'err';
      }
    })
    .catch(e => {
      status.textContent = 'Network error: ' + e;
      status.className = 'err';
    });
  }

  window.addEventListener('pagehide', saveSettings);
  window.addEventListener('beforeunload', saveSettings);

  document.addEventListener('DOMContentLoaded', initUI);
</script>
</body>
</html>`, jsArray, currentVariant, autoComplete, forceQuestMissions, showNews)
}

const panelMissionPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Panel Missions</title>
<style>
  body { margin:0; padding:48px 20px; font-family:"Noto Sans",sans-serif;
         background:#0a0a0f; color:#d4cfc6; text-align:center; }
  h1   { font-size:1.3em; letter-spacing:.15em; color:#e8e0d0; margin-bottom:6px; }
  .sub { font-size:.75em; color:#888; margin-bottom:28px; }
  .sep { width:60px; border:none; border-top:1px solid #333; margin:24px auto; }
  p    { font-size:.85em; line-height:1.6; color:#999; max-width:340px; margin:0 auto; }
</style>
</head>
<body>
  <h1>PANEL MISSIONS</h1>
  <div class="sub">Card Stories</div>
  <hr class="sep">
  <p>All panel missions are cleared.</p>
  <p>Their Card Stories are available in Library &rsaquo; Extra Stories.</p>
</body>
</html>`

func renderGachaPage(gachaID, path string) string {
	title := "GACHA DETAILS"
	description := fmt.Sprintf("Information and rates for Gacha ID: %s", gachaID)

	switch gachaID {
	case "168":
		title = "PREMIUM GACHA"
		description = "Featured characters and weapon rates for Banner 168."
	case "200000", "200001":
		title = "BOX GACHA"
		description = "Item pool and reward availability for Box Gacha."
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>
  body { margin:0; padding:40px 20px; font-family:"Noto Sans",sans-serif;
         background:#0a0a0f; color:#d4cfc6; text-align:center; }
  h1   { font-size:1.3em; letter-spacing:.15em; color:#e8e0d0; margin-bottom:6px; }
  .sub { font-size:.75em; color:#888; margin-bottom:28px; }
  .sep { width:60px; border:none; border-top:1px solid #333; margin:24px auto; }
  p    { font-size:.85em; line-height:1.6; color:#999; max-width:340px; margin:0 auto 12px; }
  .id-tag { font-size:.7em; color:#666; font-family:monospace; }
</style>
</head>
<body>
  <h1>%s</h1>
  <div class="sub">Private Server Details</div>
  <hr class="sep">
  <p>%s</p>
  <hr class="sep">
  <div class="id-tag">Gacha ID: %s</div>
</body>
</html>`, title, title, description, gachaID)
}

const resourcesURLOriginal = "https://resources.app.nierreincarnation.com"

type OctoHTTPServer struct {
	mux              *http.ServeMux
	ResourcesBaseURL string
	BaseDir          string
	users            store.UserRepository
	revisions        *revisionTracker
	resolver         *assetResolver
}

func staticPageLanguage(path string) string {
	parts := strings.Split(path, "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "static" && parts[i+1] != "" {
			return parts[i+1]
		}
	}
	return "unknown"
}

func renderStaticTermsPage(title, language, version string) string {
	return "<html><head><title>" + title + "</title></head><body><h1>" + title +
		"</h1><p>Language: " + language + "</p><p>Version: " + version + "</p></body></html>"
}

type countResponseWriter struct {
	http.ResponseWriter
	n int64
}

type fileMD5Entry struct {
	size    int64
	modTime int64
	md5     string
}

var (
	fileMD5Cache   = make(map[string]fileMD5Entry)
	fileMD5CacheMu sync.RWMutex
)

func (c *countResponseWriter) Write(p []byte) (int, error) {
	n, err := c.ResponseWriter.Write(p)
	c.n += int64(n)
	return n, err
}

func fileMD5Hex(path string, info os.FileInfo) (string, error) {
	modTime := info.ModTime().UnixNano()

	fileMD5CacheMu.RLock()
	cached, ok := fileMD5Cache[path]
	fileMD5CacheMu.RUnlock()
	if ok && cached.size == info.Size() && cached.modTime == modTime {
		return cached.md5, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	sum := hex.EncodeToString(h.Sum(nil))

	fileMD5CacheMu.Lock()
	fileMD5Cache[path] = fileMD5Entry{
		size:    info.Size(),
		modTime: modTime,
		md5:     sum,
	}
	fileMD5CacheMu.Unlock()
	return sum, nil
}

func NewOctoHTTPServer(resourcesBaseURL, baseDir string, users store.UserRepository) *OctoHTTPServer {
	s := &OctoHTTPServer{
		mux:              http.NewServeMux(),
		ResourcesBaseURL: resourcesBaseURL,
		BaseDir:          baseDir,
		users:            users,
		revisions:        newRevisionTracker(),
		resolver:         newAssetResolver(baseDir),
	}
	s.resolver.Prewarm("0", platformAndroid)
	s.resolver.Prewarm("0", platformIOS)
	s.resolver.Prewarm("0", "")
	s.mux.HandleFunc("/", s.handleAll)
	return s
}

func (s *OctoHTTPServer) Handler() http.Handler {
	return s.mux
}

func (s *OctoHTTPServer) listBinPath(revision, platform string) string {
	p := filepath.Join(s.BaseDir, "assets", "revisions", revision, platform, "list.bin")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return filepath.Join(s.BaseDir, "assets", "revisions", revision, "list.bin")
}

func (s *OctoHTTPServer) handleAll(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	platform := platformFromUserAgent(r)
	isAssetRequest := strings.Contains(path, "/unso-")
	isMasterDataRequest := strings.Contains(path, "/assets/release/") && strings.Contains(path, "database.bin")
	if !isAssetRequest && !isMasterDataRequest {
		log.Printf("[HTTP] %s %s (Host: %s, platform: %s)", r.Method, r.URL.String(), r.Host, platform)
		for k, v := range r.Header {
			log.Printf("[HTTP]   %s: %s", k, v)
		}
	}

	// Endpoint that saves settings when the page closes
	if strings.HasPrefix(path, "/information/save") && r.Method == http.MethodPost {
		userID := extractUserID(r)
		var settings UserSettings
		if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := s.saveUserSettings(userID, settings); err != nil {
			log.Printf("[DB] %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	
		// Admin: send item to user's mailbox
	if path == "/information/admin/gift" && r.Method == http.MethodPost {
		var body struct {
			Line string `json:"line"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"invalid json"}`))
			return
		}
		parts := strings.Fields(body.Line)
		if len(parts) != 4 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"expected 4 numbers: userId possessionType possessionId count"}`))
			return
		}
		userId, err1 := strconv.ParseInt(parts[0], 10, 64)
		possType, err2 := strconv.ParseInt(parts[1], 10, 32)
		possId, err3 := strconv.ParseInt(parts[2], 10, 32)
		count, err4 := strconv.ParseInt(parts[3], 10, 32)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || userId <= 0 || count <= 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"invalid numbers"}`))
			return
		}

		nowMillis := gametime.NowMillis()
		const giftExpiry = int64(30 * 24 * time.Hour / time.Millisecond) // 30 дней

		_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
			user.AddGift(store.NotReceivedGiftState{
				GiftCommon: store.GiftCommonState{
					PossessionType: int32(possType),
					PossessionId:   int32(possId),
					Count:          int32(count),
					GrantDatetime:  nowMillis,
				},
				ExpirationDatetime: nowMillis + giftExpiry,
				UserGiftUuid:       uuid.New().String(),
			})
		})
		if err != nil {
			log.Printf("[AdminGift] UpdateUser %d: %v", userId, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"error":%q}`, err.Error())
			return
		}

		log.Printf("[AdminGift] mailed userId=%d type=%d id=%d count=%d", userId, possType, possId, count)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"message":"sent to user %d: type=%d id=%d x%d"}`, userId, possType, possId, count)
		return
	}

	// Octo v2 API
	if strings.HasPrefix(path, "/v2/") {
		s.handleOctoV2(w, r, path, platform)
		return
	}

	// Octo v1 list
	if strings.HasPrefix(path, "/v1/list/") {
		s.serveOctoV1List(w, r, path, platform)
		return
	}

	// Game web API requests
	if strings.Contains(path, "/web/") || strings.Contains(r.Host, "web.app.nierreincarnation") {
		s.handleWebAPI(w, r, path)
		return
	}

	// Master data download
	if strings.HasPrefix(path, "/master-data/") {
		log.Printf("[HTTP] Master data request for path: %s — returning empty", path)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(200)
		return
	}

	// /assets/release/{version}/database.bin.e
	if strings.Contains(path, "/assets/release/") && strings.Contains(path, "database.bin") {
		s.serveDatabaseBinE(w, r, path)
		return
	}

	// Asset bundle requests
	if strings.Contains(path, "/unso-") {
		s.serveUnsoAsset(w, r, path, platform)
		return
	}

	// In-game information / news page
	if strings.Contains(path, "/information") {
		userID := extractUserID(r)
		variants := s.getMasterDataVariants()
		dbSettings := s.getUserSettings(userID)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte(renderInformationPage(variants, dbSettings.MasterdataVariant, dbSettings.AutoComplete, dbSettings.ForceQuestMissions, dbSettings.ShowNews)))
		return
	}

	// Log request body
	if r.Body != nil {
		body := make([]byte, 4096)
		n, _ := r.Body.Read(body)
		if n > 0 {
			log.Printf("[HTTP]   body (%d bytes): %x", n, body[:n])
			if n < 256 {
				log.Printf("[HTTP]   body (ascii): %s", string(body[:n]))
			}
		}
	}

	log.Printf("[HTTP] >>> UNHANDLED REQUEST: %s %s — returning empty 200", r.Method, path)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(200)
	w.Write([]byte{})
}

func (s *OctoHTTPServer) handleOctoV2(w http.ResponseWriter, r *http.Request, path, platform string) {
	log.Printf("[OctoV2] %s %s (platform=%s)", r.Method, path, platform)

	if strings.Contains(path, "/list/") {
		parts := strings.Split(path, "/")
		if len(parts) > 0 {
			requestedRevision := parts[len(parts)-1]
			if requestedRevision != "" {
				revision := "0"
				filePath := s.listBinPath(revision, platform)
				if requestedRevision != revision {
					log.Printf("[OctoV2] Resource list request revision=%s canonicalized to revision=%s", requestedRevision, revision)
				}
				log.Printf("[OctoV2] Resource list request — serving %s (requested_revision=%s canonical_revision=%s platform=%s)", filePath, requestedRevision, revision, platform)
				s.revisions.Remember(r.RemoteAddr, revision)
				go s.resolver.Prewarm(revision, platform)
				s.serveListBin(w, filePath)
				return
			}
		}

		log.Printf("[OctoV2] Resource list request without revision segment — returning empty protobuf")
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
		return
	}

	if strings.Contains(path, "/info") {
		log.Printf("[OctoV2] Info request — returning empty protobuf")
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(200)
		return
	}

	log.Printf("[OctoV2] Unknown endpoint: %s — returning empty protobuf", path)
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(200)
}

func (s *OctoHTTPServer) serveOctoV1List(w http.ResponseWriter, r *http.Request, path, platform string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	requestedRevision := "0"
	if len(parts) >= 4 {
		requestedRevision = parts[len(parts)-1]
	}
	revision := "0"
	filePath := s.listBinPath(revision, platform)
	if requestedRevision != revision {
		log.Printf("[OctoV1] list request revision=%s canonicalized to revision=%s", requestedRevision, revision)
	}
	log.Printf("[OctoV1] %s %s — serving %s (requested_revision=%s canonical_revision=%s platform=%s)", r.Method, path, filePath, requestedRevision, revision, platform)
	s.revisions.Remember(r.RemoteAddr, revision)
	go s.resolver.Prewarm(revision, platform)
	s.serveListBin(w, filePath)
}

func (s *OctoHTTPServer) serveUnsoAsset(w http.ResponseWriter, r *http.Request, path, platform string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	var segment, objectId string
	for i, p := range parts {
		if strings.HasPrefix(p, "unso-") && i+1 < len(parts) {
			segment = p
			objectId = parts[i+1]
			break
		}
	}
	if segment == "" || objectId == "" {
		log.Printf("[HTTP] Asset request malformed: %s", path)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	segParts := strings.Split(segment, "-")
	if len(segParts) < 2 {
		log.Printf("[HTTP] Asset request segment malformed: %s", segment)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	assetType := segParts[len(segParts)-1]
	if assetType != "assetbundle" && assetType != "resources" {
		log.Printf("[HTTP] Asset request unknown type: %s", assetType)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	activeRevision := s.revisions.Active(r.RemoteAddr)
	resolution, ok := s.resolver.Resolve(objectId, assetType, activeRevision, platform)
	if !ok {
		log.Printf("[HTTP] Asset not found: %s (object_id=%s type=%s platform=%s active_revision=%s) no candidates", path, objectId, assetType, platform, activeRevision)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	revDir := filepath.Join(s.BaseDir, "assets", "revisions")
	var triedPaths []string
	var md5Mismatches []string
	for _, candidate := range resolution.Candidates {
		rel, err := filepath.Rel(revDir, candidate.Path)
		if err != nil || strings.Contains(rel, "..") || filepath.IsAbs(rel) {
			continue
		}
		triedPaths = append(triedPaths, candidate.Revision+":"+candidate.Path+" ["+candidate.Source+"]")
		f, err := os.Open(candidate.Path)
		if err != nil {
			continue
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			continue
		}
		if info.IsDir() {
			f.Close()
			continue
		}
		if resolution.ListSize >= 256 && info.Size() != resolution.ListSize {
			f.Close()
			continue
		}
		if candidate.ExpectedMD5 != "" {
			actualMD5, err := fileMD5Hex(candidate.Path, info)
			if err != nil {
				log.Printf("[HTTP] Asset md5 read failed: %s err=%v", candidate.Path, err)
				f.Close()
				continue
			}
			if !strings.EqualFold(actualMD5, candidate.ExpectedMD5) {
				md5Mismatches = append(md5Mismatches, candidate.Revision+":"+candidate.Path+" ["+candidate.Source+"] expected="+candidate.ExpectedMD5+" actual="+actualMD5)
				log.Printf("[HTTP] Asset md5 mismatch: object_id=%s type=%s platform=%s path=%s expected=%s actual=%s active_revision=%s list_revision=%s resolved_revision=%s source=%s", objectId, assetType, platform, candidate.Path, candidate.ExpectedMD5, actualMD5, resolution.ActiveRevision, resolution.ListRevision, candidate.Revision, candidate.Source)
				f.Close()
				continue
			}
		}
		defer f.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		cw := &countResponseWriter{ResponseWriter: w}
		serveStart := time.Now()
		http.ServeContent(cw, r, filepath.Base(candidate.Path), info.ModTime(), f)
		log.Printf("[HTTP] Asset served: object_id=%s type=%s platform=%s bytes=%d elapsed=%s path=%s", objectId, assetType, platform, cw.n, time.Since(serveStart), candidate.Path)
		return
	}
	if len(md5Mismatches) > 0 {
		log.Printf("[HTTP] Asset md5 mismatches: object_id=%s type=%s platform=%s active_revision=%s list_revision=%s mismatches=%v", objectId, assetType, platform, resolution.ActiveRevision, resolution.ListRevision, md5Mismatches)
	}
	log.Printf("[HTTP] Asset not found: %s (object_id=%s type=%s platform=%s active_revision=%s list_revision=%s) tried paths: %v", path, objectId, assetType, platform, resolution.ActiveRevision, resolution.ListRevision, triedPaths)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusNotFound)
}

func (s *OctoHTTPServer) serveListBin(w http.ResponseWriter, filePath string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("[Octo] list.bin read error: %v", err)
		http.Error(w, "list not found", http.StatusNotFound)
		return
	}
	orig := []byte(resourcesURLOriginal)
	if s.ResourcesBaseURL != "" {
		if len(s.ResourcesBaseURL) != len(orig) {
			log.Printf("[Octo] resources-base-url length is %d, need %d — serving list.bin unchanged", len(s.ResourcesBaseURL), len(orig))
		} else {
			repl := []byte(s.ResourcesBaseURL)
			if idx := bytes.Index(data, orig); idx >= 0 {
				copy(data[idx:], repl)
				log.Printf("[Octo] list.bin: rewrote resource base URL to %s", s.ResourcesBaseURL)
			}
		}
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (s *OctoHTTPServer) serveDatabaseBinE(w http.ResponseWriter, r *http.Request, path string) {
	fileName := "20240404193219.bin.e"
	const masterDataBaseVersion = "20240404193219"

	versionParts := strings.Split(path, "/")
	for _, part := range versionParts {
		if strings.HasPrefix(part, masterDataBaseVersion) {
			underscores := strings.Split(part, "_")
			if len(underscores) >= 3 {
				lastPart := underscores[len(underscores)-1]
				if userdata.IsValidMasterDataVariant(lastPart) {
					fileName = fmt.Sprintf("20240404193219_%s.bin.e", lastPart)
					log.Printf("[MasterData] Using variant file: %s", fileName)
				}
			}
			break
		}
	}

	log.Printf("[MasterData] Serving file: %s", fileName)
	filePath := filepath.Join(s.BaseDir, "assets", "release", fileName)
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, filePath)
}

func (s *OctoHTTPServer) handleWebAPI(w http.ResponseWriter, r *http.Request, path string) {
	log.Printf("[WebAPI] Serving: %s", path)

	if strings.Contains(path, "database.bin") {
		s.serveDatabaseBinE(w, r, path)
		return
	}

	if strings.Contains(path, "termsofuse") {
		language := staticPageLanguage(path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(200)
		w.Write([]byte(renderStaticTermsPage("Terms of Service", language, termsVersionMarker)))
		return
	}

	if strings.Contains(path, "privacy") {
		language := staticPageLanguage(path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte(renderStaticTermsPage("Privacy Policy", language, privacyVersionMarker)))
		return
	}

	if strings.Contains(path, "maintenance") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte(`<!DOCTYPE html><html><body></body></html>`))
		return
	}

	if strings.Contains(path, "panelmission") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte(panelMissionPage))
		return
	}

	if strings.Contains(path, "gacha") {
		gachaID := r.URL.Query().Get("gachaId")
		if gachaID == "" {
			gachaID = "Unknown"
		}

		htmlContent := renderGachaPage(gachaID, path)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte(htmlContent))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte(`<!DOCTYPE html><html><body></body></html>`))
}
