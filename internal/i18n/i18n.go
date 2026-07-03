package i18n

import "strings"

var translations = map[string]map[string]string{
	// Navigation
	"nav.shows":    {"es": "Series", "en": "Shows"},
	"nav.movies":   {"es": "Películas", "en": "Movies"},
	"nav.upcoming": {"es": "Próximos", "en": "Upcoming"},
	"nav.lists":    {"es": "Listas", "en": "Lists"},
	"nav.stats":    {"es": "Stats", "en": "Stats"},
	"nav.search":   {"es": "Buscar", "en": "Search"},
	"nav.add":      {"es": "+ Añadir", "en": "+ Add"},

	// Footer
	"footer.tagline": {"es": "WatchLog — Tu seguimiento personal de series y películas", "en": "WatchLog — Your personal TV & movie tracker"},

	// Dashboard
	"dashboard.title":          {"es": "Tu colección", "en": "Your collection"},
	"dashboard.subtitle":       {"es": "Seguimiento personal", "en": "Personal tracking"},
	"dashboard.shows":          {"es": "Series", "en": "Shows"},
	"dashboard.episodes":       {"es": "Episodios", "en": "Episodes"},
	"dashboard.movies":         {"es": "Películas", "en": "Movies"},
	"dashboard.total_time":     {"es": "Tiempo total", "en": "Total time"},
	"dashboard.shows_link":     {"es": "Series →", "en": "Shows →"},
	"dashboard.movies_link":    {"es": "Películas →", "en": "Movies →"},
	"dashboard.upcoming_link":  {"es": "Próximos →", "en": "Upcoming →"},

	// Shows
	"shows.title":         {"es": "Series", "en": "Shows"},
	"shows.titles_count":  {"es": "títulos", "en": "titles"},
	"shows.sort":          {"es": "Ordenar:", "en": "Sort:"},
	"shows.sort.recent":   {"es": "Recientes", "en": "Recent"},
	"shows.sort.name":     {"es": "A–Z", "en": "A–Z"},
	"shows.sort.episodes": {"es": "Más vistos", "en": "Most watched"},
	"shows.sort.followed": {"es": "Fecha seguimiento", "en": "Follow date"},
	"shows.no_image":      {"es": "Sin imagen", "en": "No image"},
	"shows.episodes":      {"es": "episodios", "en": "episodes"},

	// Show detail
	"show.back":             {"es": "← Volver", "en": "← Back"},
	"show.episodes_watched": {"es": "Episodios vistos", "en": "Episodes watched"},
	"show.seasons":          {"es": "Temporadas", "en": "Seasons"},
	"show.last":             {"es": "Último:", "en": "Last:"},
	"show.mark_next":        {"es": "Marcar siguiente", "en": "Mark next"},
	"show.mark":             {"es": "Marcar", "en": "Mark"},
	"show.following":        {"es": "Siguiendo", "en": "Following"},
	"show.follow":           {"es": "Seguir", "en": "Follow"},
	"show.favorite":         {"es": "Favorita", "en": "Favorite"},
	"show.archive":          {"es": "Archivar", "en": "Archive"},
	"show.episodes_by_season": {"es": "Episodios por temporada", "en": "Episodes by season"},
	"show.season":           {"es": "Temporada", "en": "Season"},
	"show.mark_all":         {"es": "Marcar toda →", "en": "Mark all →"},
	"show.mark_manually":    {"es": "Marcar manualmente", "en": "Mark manually"},

	// Movies
	"movies.title":       {"es": "Películas", "en": "Movies"},
	"movies.sort":        {"es": "Ordenar:", "en": "Sort:"},
	"movies.sort.recent": {"es": "Recientes", "en": "Recent"},
	"movies.sort.name":   {"es": "A–Z", "en": "A–Z"},

	// Stats
	"stats.title":     {"es": "Estadísticas", "en": "Statistics"},
	"stats.shows":     {"es": "Series", "en": "Shows"},
	"stats.episodes":  {"es": "Episodios", "en": "Episodes"},
	"stats.movies":    {"es": "Películas", "en": "Movies"},
	"stats.time":      {"es": "Tiempo", "en": "Time"},
	"stats.calendar":  {"es": "Calendario de actividad", "en": "Activity calendar"},
	"stats.less":      {"es": "Menos", "en": "Less"},
	"stats.more":      {"es": "Más", "en": "More"},

	// Lists
	"lists.title":       {"es": "Listas", "en": "Lists"},
	"lists.new":         {"es": "Nueva lista...", "en": "New list..."},
	"lists.create":      {"es": "Crear", "en": "Create"},
	"lists.empty":       {"es": "No tienes listas todavía", "en": "No lists yet"},
	"lists.back":        {"es": "← Volver", "en": "← Back"},
	"lists.public":      {"es": "Pública", "en": "Public"},
	"lists.delete":      {"es": "Eliminar", "en": "Delete"},
	"lists.remove_item": {"es": "Quitar", "en": "Remove"},
	"lists.empty_list":  {"es": "Lista vacía", "en": "Empty list"},
	"lists.add_to":      {"es": "Añadir a la lista", "en": "Add to list"},
	"lists.search":      {"es": "Buscar serie o película...", "en": "Search show or movie..."},
	"lists.confirm_delete": {"es": "¿Eliminar esta lista?", "en": "Delete this list?"},

	// Search
	"search.placeholder": {"es": "Buscar", "en": "Search"},
	"search.searching":   {"es": "Buscando...", "en": "Searching..."},
	"search.no_results":  {"es": "Sin resultados para", "en": "No results for"},
	"search.shows":       {"es": "Series", "en": "Shows"},
	"search.movies":      {"es": "Películas", "en": "Movies"},

	// Add
	"add.title":       {"es": "Añadir", "en": "Add"},
	"add.shows":       {"es": "Series", "en": "Shows"},
	"add.movies":      {"es": "Películas", "en": "Movies"},
	"add.search_tmdb": {"es": "Buscar en TMDB", "en": "Search TMDB"},
	"add.add":         {"es": "Añadir", "en": "Add"},
	"add.no_results":  {"es": "Sin resultados", "en": "No results"},

	// Upcoming
	"upcoming.title":    {"es": "Próximos episodios", "en": "Upcoming episodes"},
	"upcoming.subtitle": {"es": "Series en emisión que sigues · se actualiza automáticamente cada día", "en": "Airing shows you follow · auto-updates daily"},
	"upcoming.empty":    {"es": "No hay episodios próximos programados", "en": "No upcoming episodes scheduled"},
	"upcoming.cache":    {"es": "La caché se renueva automáticamente cada día", "en": "Cache refreshes automatically every day"},

	// Auth
	"login.title":       {"es": "Iniciar sesión", "en": "Log in"},
	"login.username":    {"es": "Usuario", "en": "Username"},
	"login.password":    {"es": "Contraseña", "en": "Password"},
	"login.submit":      {"es": "Entrar", "en": "Log in"},
	"login.no_account":  {"es": "¿No tienes cuenta?", "en": "Don't have an account?"},
	"login.register":    {"es": "Registrarse", "en": "Sign up"},
	"login.error":       {"es": "Usuario o contraseña incorrectos", "en": "Invalid username or password"},
	"login.rate_limited": {"es": "Demasiados intentos. Espera unos minutos.", "en": "Too many attempts. Please wait a few minutes."},
	"register.title":    {"es": "Crear cuenta", "en": "Create account"},
	"register.submit":   {"es": "Crear cuenta", "en": "Create account"},
	"register.has_account": {"es": "¿Ya tienes cuenta?", "en": "Already have an account?"},
	"register.login":    {"es": "Iniciar sesión", "en": "Log in"},
	"register.error.username_required": {"es": "El nombre de usuario es obligatorio", "en": "Username is required"},
	"register.error.password_min":      {"es": "La contraseña debe tener al menos 8 caracteres", "en": "Password must be at least 8 characters"},
	"register.error.internal":          {"es": "Error interno", "en": "Internal error"},
	"register.error.username_taken":    {"es": "El nombre de usuario ya existe", "en": "Username already exists"},

	// Setup
	"setup.error.username_required": {"es": "El nombre de usuario es obligatorio", "en": "Username is required"},
	"setup.error.password_min":      {"es": "La contraseña debe tener al menos 8 caracteres", "en": "Password must be at least 8 characters"},
	"setup.error.create_failed":     {"es": "No se pudo crear el usuario", "en": "Could not create user"},

	// TMDB inline messages
	"tmdb.not_configured":     {"es": "TMDB no configurado", "en": "TMDB not configured"},
	"tmdb.added":              {"es": "añadida", "en": "added"},
	"tmdb.list_added":         {"es": "Añadido", "en": "Added"},
	"tmdb.season_marked":      {"es": "Temporada %d: %d episodios", "en": "Season %d: %d episodes"},
	"tmdb.upcoming_refreshed": {"es": "Próximos episodios actualizados", "en": "Upcoming episodes refreshed"},
	"tmdb.refresh_result":     {"es": "Actualizado: %d series, %d películas", "en": "Updated: %d shows, %d movies"},
	"tmdb.fetch_result":       {"es": "Series: %d/%d, Películas: %d/%d", "en": "Shows: %d/%d, Movies: %d/%d"},

	// Settings
	"settings.title":    {"es": "Ajustes", "en": "Settings"},
	"settings.language": {"es": "Idioma", "en": "Language"},
	"settings.save":     {"es": "Guardar", "en": "Save"},
	"settings.saved":           {"es": "Guardado", "en": "Saved"},
	"settings.current_key":     {"es": "Clave actual", "en": "Current key"},
	"settings.tmdb_placeholder": {"es": "Introduce nueva clave para actualizar", "en": "Enter new key to update"},
	"settings.tmdb_help":       {"es": "Déjalo vacío para mantener la clave actual. Obtén una en themoviedb.org/settings/api", "en": "Leave empty to keep current key. Get one at themoviedb.org/settings/api"},
	"settings.refresh_metadata": {"es": "Refrescar metadatos", "en": "Refresh metadata"},
	"settings.refresh_upcoming": {"es": "Refrescar próximos", "en": "Refresh upcoming"},

	// Import
	"import.title":       {"es": "Importar datos", "en": "Import data"},
	"import.subtitle":    {"es": "Sube el archivo ZIP de exportación de TVTime", "en": "Upload your TVTime export ZIP file"},
	"import.select_file": {"es": "Seleccionar archivo", "en": "Select file"},
	"import.start":       {"es": "Iniciar importación", "en": "Start import"},
	"import.log":         {"es": "Progreso", "en": "Progress"},
	"import.go_home":     {"es": "Ir al inicio", "en": "Go to home"},
	"import.link":        {"es": "Importar datos TVTime", "en": "Import TVTime data"},
	"import.starting":    {"es": "Iniciando importación...", "en": "Starting import..."},
	"import.complete":    {"es": "✓ Importación completada!", "en": "✓ Import complete!"},
	"import.fetching_tmdb":       {"es": "Obteniendo metadatos TMDB...", "en": "Fetching TMDB metadata..."},
	"import.tmdb_complete":       {"es": "✓ Metadatos TMDB obtenidos!", "en": "✓ TMDB fetch complete!"},
	"import.refreshing_upcoming": {"es": "Actualizando próximos episodios...", "en": "Refreshing upcoming episodes..."},
	"import.upcoming_complete":   {"es": "✓ Próximos episodios actualizados!", "en": "✓ Upcoming episodes updated!"},
}

// T returns the translation for a given key and language.
// Falls back to Spanish if the key or language is not found.
func T(lang, key string) string {
	t, ok := translations[key]
	if !ok {
		return key
	}
	s, ok := t[lang]
	if !ok {
		return t["es"]
	}
	return s
}

// DetectLang returns the preferred language from Accept-Language header.
// Returns "en" or "es" (default).
func DetectLang(acceptLang string) string {
	lower := strings.ToLower(acceptLang)
	// Simple parsing: check if "en" appears before "es"
	enIdx := strings.Index(lower, "en")
	esIdx := strings.Index(lower, "es")
	if enIdx >= 0 && (esIdx < 0 || enIdx < esIdx) {
		return "en"
	}
	return "es"
}

// genreTranslations is intentionally empty — genre translations come from TMDB.
// For TV shows where TMDB doesn't translate, we display as-is from the API.
// This function exists as a hook in case local overrides are needed in the future.
func TranslateGenres(lang, genres string) string {
	return genres
}
