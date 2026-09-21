package theme

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/platform/marketutil"
	"github.com/komari-monitor/komari/internal/platform/respond"
)

func ListThemeMarketSources(c *gin.Context) {
	sources, err := getThemeMarketSources()
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "Failed to load theme market sources: "+err.Error())
		return
	}
	respond.Success(c, sources)
}

func CreateThemeMarketSource(c *gin.Context) {
	var source ThemeMarketSource
	if err := c.ShouldBindJSON(&source); err != nil {
		respond.Error(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}
	var err error
	source, err = normalizeThemeMarketSource(source)
	if err != nil {
		respond.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	source.ID, err = marketutil.NewSourceID()
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "Failed to create source ID")
		return
	}
	sources, err := getThemeMarketSources()
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "Failed to load theme market sources: "+err.Error())
		return
	}
	for _, existing := range sources {
		if existing.URL == source.URL {
			respond.Error(c, http.StatusConflict, "A source with this URL already exists")
			return
		}
	}
	sources = append(sources, source)
	if err := saveThemeMarketSources(sources); err != nil {
		respond.Error(c, http.StatusInternalServerError, "Failed to save theme market source: "+err.Error())
		return
	}
	respond.SuccessMessage(c, "Theme market source created", source)
}

func UpdateThemeMarketSource(c *gin.Context) {
	var update ThemeMarketSource
	if err := c.ShouldBindJSON(&update); err != nil {
		respond.Error(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}
	update.ID = c.Param("id")
	var err error
	update, err = normalizeThemeMarketSource(update)
	if err != nil {
		respond.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	sources, err := getThemeMarketSources()
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "Failed to load theme market sources: "+err.Error())
		return
	}
	found := false
	oldURL := ""
	for i := range sources {
		if sources[i].ID == update.ID {
			oldURL = sources[i].URL
			sources[i] = update
			found = true
			continue
		}
		if sources[i].URL == update.URL {
			respond.Error(c, http.StatusConflict, "A source with this URL already exists")
			return
		}
	}
	if !found {
		respond.Error(c, http.StatusNotFound, "Theme market source not found")
		return
	}
	if err := saveThemeMarketSources(sources); err != nil {
		respond.Error(c, http.StatusInternalServerError, "Failed to save theme market source: "+err.Error())
		return
	}
	invalidateThemeMarketCache(oldURL)
	if oldURL != update.URL {
		invalidateThemeMarketCache(update.URL)
	}
	respond.SuccessMessage(c, "Theme market source updated", update)
}

func DeleteThemeMarketSource(c *gin.Context) {
	id := c.Param("id")
	sources, err := getThemeMarketSources()
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "Failed to load theme market sources: "+err.Error())
		return
	}
	next := make([]ThemeMarketSource, 0, len(sources))
	var deleted *ThemeMarketSource
	for i := range sources {
		if sources[i].ID == id {
			removed := sources[i]
			deleted = &removed
			continue
		}
		next = append(next, sources[i])
	}
	if deleted == nil {
		respond.Error(c, http.StatusNotFound, "Theme market source not found")
		return
	}
	if err := saveThemeMarketSources(next); err != nil {
		respond.Error(c, http.StatusInternalServerError, "Failed to delete theme market source: "+err.Error())
		return
	}
	invalidateThemeMarketCache(deleted.URL)
	respond.SuccessMessage(c, "Theme market source deleted", nil)
}

func ListThemeMarketCatalog(c *gin.Context) {
	sources, err := getThemeMarketSources()
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "Failed to load theme market sources: "+err.Error())
		return
	}
	force := c.Query("refresh") == "true"
	themes := make([]ThemeMarketTheme, 0)
	statuses := make([]themeMarketSourceStatus, len(sources))
	results := make([][]ThemeMarketTheme, len(sources))
	var wg sync.WaitGroup
	for i, source := range sources {
		statuses[i] = themeMarketSourceStatus{ID: source.ID, Name: source.Name, URL: source.URL}
		if !source.Enabled {
			continue
		}
		wg.Add(1)
		go func(index int, item ThemeMarketSource) {
			defer wg.Done()
			items, fetchErr := fetchThemeMarketCatalog(item, force)
			if fetchErr != nil {
				statuses[index].Error = fetchErr.Error()
				return
			}
			results[index] = items
			statuses[index].Count = len(items)
		}(i, source)
	}
	wg.Wait()
	for _, items := range results {
		themes = append(themes, items...)
	}
	respond.Success(c, gin.H{"themes": themes, "sources": statuses})
}

func InstallThemeFromMarket(c *gin.Context) {
	var req struct {
		SourceID string `json:"source_id" binding:"required"`
		Short    string `json:"short" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respond.Error(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}
	sources, err := getThemeMarketSources()
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "Failed to load theme market sources: "+err.Error())
		return
	}
	var source *ThemeMarketSource
	for i := range sources {
		if sources[i].ID == req.SourceID && sources[i].Enabled {
			source = &sources[i]
			break
		}
	}
	if source == nil {
		respond.Error(c, http.StatusNotFound, "Theme market source not found or disabled")
		return
	}
	items, err := fetchThemeMarketCatalog(*source, true)
	if err != nil {
		respond.Error(c, http.StatusBadRequest, "Failed to load theme market source: "+err.Error())
		return
	}
	var selected *ThemeMarketTheme
	for i := range items {
		if items[i].Short == req.Short {
			selected = &items[i]
			break
		}
	}
	if selected == nil {
		respond.Error(c, http.StatusNotFound, "Theme not found in source")
		return
	}
	if !selected.Installable {
		respond.Error(c, http.StatusBadRequest, "This theme does not provide an installable package")
		return
	}
	data, err := marketutil.DownloadMarketURL(selected.Download, marketutil.PackageMaxSize)
	if err != nil {
		respond.Error(c, http.StatusBadRequest, "Failed to download theme: "+err.Error())
		return
	}
	digest := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), selected.SHA256) {
		respond.Error(c, http.StatusBadRequest, "Theme SHA-256 checksum does not match the market catalog")
		return
	}
	tempFile, err := os.CreateTemp("", "komari-market-theme-*.zip")
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "Failed to create temporary theme file")
		return
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		respond.Error(c, http.StatusInternalServerError, "Failed to save temporary theme file")
		return
	}
	if err := tempFile.Close(); err != nil {
		respond.Error(c, http.StatusInternalServerError, "Failed to save temporary theme file")
		return
	}
	manifest, err := peekThemeFromZip(tempPath)
	if err != nil {
		respond.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if manifest.Short != selected.Short || manifest.Version != selected.Version {
		respond.Error(c, http.StatusBadRequest, "Theme manifest does not match the market catalog")
		return
	}
	installed, err := InstallZip(tempPath)
	if err != nil {
		respond.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	respond.SuccessMessage(c, "Theme installed from market", installed)
}
