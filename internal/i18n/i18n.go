package i18n

import "strings"

var translations = map[string]map[string]string{
	// Navigation
	"nav.shows":    {"es": "Series", "en": "Shows"},
	"nav.movies":   {"es": "Películas", "en": "Movies"},
	"nav.upcoming": {"es": "Próximos", "en": "Upcoming"},
	"nav.lists":    {"es": "Listas", "en": "Lists"},
	"nav.stats":    {"es": "Stats", "en": "Stats"},
	"nav.timeline": {"es": "Historial", "en": "Timeline"},
	"nav.search":   {"es": "Buscar", "en": "Search"},
	"nav.add":      {"es": "+ Añadir", "en": "+ Add"},
	"nav.settings": {"es": "Ajustes", "en": "Settings"},
	"nav.admin":    {"es": "Administrar", "en": "Manage"},

	// Footer
	"footer.tagline": {"es": "WatchLog — Tu seguimiento personal de series y películas", "en": "WatchLog — Your personal TV & movie tracker"},

	// PWA
	"pwa.install_prompt": {"es": "Instalar WatchLog", "en": "Install WatchLog"},
	"pwa.install":        {"es": "Instalar", "en": "Install"},
	"pwa.ios_prompt":     {"es": "Pulsa compartir ⎋ y \"Añadir a pantalla de inicio\"", "en": "Tap share ⎋ and \"Add to Home Screen\""},

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
	"dashboard.continue":       {"es": "Continuar viendo", "en": "Continue watching"},
	"dashboard.new_seasons":    {"es": "Novedades", "en": "New seasons"},
	"dashboard.new_season_label": {"es": "Nueva temporada", "en": "New season"},
	"dashboard.mark_watched":   {"es": "Marcar como visto", "en": "Mark as watched"},
	"dashboard.snooze":         {"es": "Posponer", "en": "Snooze"},

	// Shows
	"shows.title":         {"es": "Series", "en": "Shows"},
	"shows.titles_count":  {"es": "títulos", "en": "titles"},
	"shows.sort":          {"es": "Ordenar:", "en": "Sort:"},
	"shows.sort.recent":   {"es": "Recientes", "en": "Recent"},
	"shows.sort.name":     {"es": "A–Z", "en": "A–Z"},
	"shows.sort.episodes": {"es": "Más vistos", "en": "Most watched"},
	"shows.sort.followed": {"es": "Fecha seguimiento", "en": "Follow date"},
	"shows.filter":          {"es": "Filtrar:", "en": "Filter:"},
	"shows.filter.all":      {"es": "Todas", "en": "All"},
	"shows.filter.watching": {"es": "Por ver", "en": "To watch"},
	"shows.filter.completed": {"es": "Al día", "en": "Caught up"},
	"shows.filter.favorites": {"es": "Favoritas", "en": "Favorites"},
	"shows.filter.archived": {"es": "Archivadas", "en": "Archived"},
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
	"show.rematch":          {"es": "Corregir serie (TMDB)", "en": "Fix TMDB match"},
	"show.rematch_help":     {"es": "¿Datos incorrectos? Busca la serie correcta y re-vincúlala a TMDB.", "en": "Wrong data? Search for the correct show and re-link it to TMDB."},
	"show.rematch_search":   {"es": "Buscar la serie correcta...", "en": "Search for the correct show..."},
	"show.rematch_use":      {"es": "Usar esta", "en": "Use this"},
	"show.rematch_done":     {"es": "Re-vinculada. Recargando...", "en": "Re-linked. Reloading..."},
	"show.rematch_error":    {"es": "Error al re-vincular", "en": "Re-link failed"},
	"watch.date":            {"es": "Fecha de visionado", "en": "Watched on"},
	"watch.edit_dates":      {"es": "Fechas de visionado", "en": "Watch dates"},
	"watch.edit_dates_help": {"es": "Corrige la fecha y hora de cada episodio visto.", "en": "Correct the date and time of each watched episode."},
	"watch.save":            {"es": "Guardar", "en": "Save"},
	"watch.saved":           {"es": "Guardado", "en": "Saved"},
	"watch.error":           {"es": "Error", "en": "Error"},
	"watch.edit":            {"es": "Editar fecha", "en": "Edit date"},
	"calendar.title":        {"es": "Calendario", "en": "Calendar"},
	"calendar.empty":        {"es": "Nada visto este mes", "en": "Nothing watched this month"},
	"nav.calendar":          {"es": "Calendario", "en": "Calendar"},
	"cal.d0":                {"es": "Lun", "en": "Mon"},
	"cal.d1":                {"es": "Mar", "en": "Tue"},
	"cal.d2":                {"es": "Mié", "en": "Wed"},
	"cal.d3":                {"es": "Jue", "en": "Thu"},
	"cal.d4":                {"es": "Vie", "en": "Fri"},
	"cal.d5":                {"es": "Sáb", "en": "Sat"},
	"cal.d6":                {"es": "Dom", "en": "Sun"},
	"show.episodes_by_season": {"es": "Episodios por temporada", "en": "Episodes by season"},
	"show.season":           {"es": "Temporada", "en": "Season"},
	"show.mark_all":         {"es": "Marcar toda →", "en": "Mark all →"},
	"show.mark_manually":    {"es": "Marcar manualmente", "en": "Mark manually"},

	// Movies
	"movies.title":       {"es": "Películas", "en": "Movies"},
	"movies.sort":        {"es": "Ordenar:", "en": "Sort:"},
	"movies.sort.recent": {"es": "Recientes", "en": "Recent"},
	"movies.sort.name":   {"es": "A–Z", "en": "A–Z"},
	"movies.total":       {"es": "Películas vistas", "en": "Movies watched"},
	"movies.runtime":     {"es": "Tiempo total", "en": "Total runtime"},
	"movies.watchlist":     {"es": "Pendientes", "en": "Watchlist"},
	"movies.watched_count": {"es": "vistas", "en": "watched"},
	"movies.mark_watched":  {"es": "Vista", "en": "Watched"},
	"movies.watched":       {"es": "Vista", "en": "Watched"},

	// Stats
	"stats.title":     {"es": "Estadísticas", "en": "Statistics"},
	"stats.shows":     {"es": "Series", "en": "Shows"},
	"stats.episodes":  {"es": "Episodios", "en": "Episodes"},
	"stats.movies":    {"es": "Películas", "en": "Movies"},
	"stats.time":      {"es": "Tiempo", "en": "Time"},
	"stats.calendar":  {"es": "Calendario de actividad", "en": "Activity calendar"},
	"stats.less":      {"es": "Menos", "en": "Less"},
	"stats.more":      {"es": "Más", "en": "More"},

	// Timeline
	"timeline.title": {"es": "Historial", "en": "Timeline"},
	"timeline.movie": {"es": "Película", "en": "Movie"},
	"timeline.items": {"es": "episodios", "en": "episodes"},
	"timeline.more":     {"es": "más", "en": "more"},
	"timeline.collapse": {"es": "Menos", "en": "Less"},
	"timeline.jump":     {"es": "Ir a:", "en": "Jump to:"},
	"timeline.today":    {"es": "Hoy", "en": "Today"},

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
	"login.magic_hint":  {"es": "Recibirás un enlace de acceso en tu email", "en": "You'll receive a login link via email"},
	"login.with_password": {"es": "Acceder con contraseña", "en": "Log in with password"},
	"login.with_magic":    {"es": "Acceder con enlace mágico", "en": "Log in with magic link"},
	"login.forgot_password": {"es": "¿Olvidaste tu contraseña?", "en": "Forgot your password?"},
	"login.error":       {"es": "Usuario o contraseña incorrectos", "en": "Invalid username or password"},
	"login.blocked":     {"es": "Tu cuenta está bloqueada", "en": "Your account is blocked"},
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
	"setup.subtitle":           {"es": "Configuración inicial", "en": "Initial setup"},
	"setup.step1_title":        {"es": "Paso 1: Crear cuenta de administrador", "en": "Step 1: Create admin account"},
	"setup.step2_title":        {"es": "Paso 2: Configuración del servidor", "en": "Step 2: Server configuration"},
	"setup.step3_title":        {"es": "Paso 3: Importar datos", "en": "Step 3: Import data"},
	"setup.step3_description":  {"es": "Si tienes un archivo ZIP de exportación de TVTime, puedes importarlo ahora.", "en": "If you have a TVTime export ZIP file, you can import it now."},
	"setup.confirm_password":   {"es": "Confirmar contraseña", "en": "Confirm password"},
	"setup.next":               {"es": "Siguiente", "en": "Next"},
	"setup.back":               {"es": "Anterior", "en": "Back"},
	"setup.skip":               {"es": "Saltar", "en": "Skip"},
	"setup.import":             {"es": "Importar", "en": "Import"},
	"setup.go_dashboard":       {"es": "Ir al inicio", "en": "Go to dashboard"},
	"setup.tmdb_help":          {"es": "Opcional. Necesario para posters y metadatos. Obtén una en themoviedb.org/settings/api", "en": "Optional. Required for posters and metadata. Get one at themoviedb.org/settings/api"},
	"setup.smtp_help":          {"es": "Opcional. Necesario para magic links y recuperación de contraseña.", "en": "Optional. Required for magic links and password recovery."},
	"setup.url_help":           {"es": "Opcional. URL pública para los enlaces en emails.", "en": "Optional. Public URL for email links."},
	"setup.auth_options":       {"es": "Opciones de autenticación", "en": "Authentication options"},
	"setup.error.username_required": {"es": "El nombre de usuario es obligatorio", "en": "Username is required"},
	"setup.error.email_required":    {"es": "El email es obligatorio", "en": "Email is required"},
	"setup.error.password_min":      {"es": "La contraseña debe tener al menos 8 caracteres", "en": "Password must be at least 8 characters"},
	"setup.error.password_mismatch": {"es": "Las contraseñas no coinciden", "en": "Passwords do not match"},
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
	"settings.theme":          {"es": "Tema", "en": "Theme"},
	"settings.theme.system":   {"es": "Sistema", "en": "System"},
	"settings.theme.light":    {"es": "Claro", "en": "Light"},
	"settings.theme.dark":     {"es": "Oscuro", "en": "Dark"},
	"settings.save":     {"es": "Guardar", "en": "Save"},
	"settings.saved":           {"es": "Guardado", "en": "Saved"},
	"settings.current_key":     {"es": "Clave actual", "en": "Current key"},
	"settings.tmdb_placeholder": {"es": "Introduce nueva clave para actualizar", "en": "Enter new key to update"},
	"settings.tmdb_help":       {"es": "Déjalo vacío para mantener la clave actual. Obtén una en themoviedb.org/settings/api", "en": "Leave empty to keep current key. Get one at themoviedb.org/settings/api"},
	"settings.refresh_metadata": {"es": "Refrescar metadatos", "en": "Refresh metadata"},
	"settings.refresh_upcoming": {"es": "Refrescar próximos", "en": "Refresh upcoming"},
	"settings.admin_link":       {"es": "Administración del servidor", "en": "Server administration"},
	"settings.logout":           {"es": "Cerrar sesión", "en": "Log out"},
	"settings.api_keys":         {"es": "API Keys (MCP)", "en": "API Keys (MCP)"},
	"settings.key_name":         {"es": "Nombre", "en": "Name"},
	"settings.key_scopes":       {"es": "Permisos", "en": "Scopes"},
	"settings.create_key":       {"es": "Crear clave", "en": "Create key"},
	"settings.confirm_delete_key": {"es": "¿Eliminar esta clave?", "en": "Delete this key?"},

	// Admin
	"admin.title":          {"es": "Administración", "en": "Administration"},
	"admin.subtitle":       {"es": "Configuración del servidor (solo admin)", "en": "Server configuration (admin only)"},
	"admin.smtp_section":   {"es": "Correo (SMTP)", "en": "Email (SMTP)"},
	"admin.smtp_help":      {"es": "Necesario para magic links y recuperación de contraseña", "en": "Required for magic links and password recovery"},
	"admin.smtp_url_help":  {"es": "Formato: smtps://usuario:contraseña@host:puerto/remitente@ejemplo.com", "en": "Format: smtps://user:password@host:port/from@example.com"},
	"admin.server_section": {"es": "Servidor", "en": "Server"},
	"admin.watchlog_url":   {"es": "URL pública", "en": "Public URL"},
	"admin.watchlog_url_help": {"es": "URL base para magic links (ej: https://watchlog.example.com)", "en": "Base URL for magic links (e.g. https://watchlog.example.com)"},
	"admin.tmdb_actions":   {"es": "Acciones TMDB", "en": "TMDB Actions"},
	"admin.loading":        {"es": "Procesando...", "en": "Processing..."},
	"admin.auth_section":    {"es": "Autenticación", "en": "Authentication"},
	"admin.auth_registration": {"es": "Permitir registro de nuevos usuarios", "en": "Allow new user registration"},
	"admin.auth_password":   {"es": "Permitir login con contraseña", "en": "Allow password login"},
	"admin.auth_magic_link": {"es": "Permitir login con enlace mágico", "en": "Allow magic link login"},
	"admin.auth_default":    {"es": "Método por defecto", "en": "Default method"},

	// Admin: user management
	"admin.users_section":       {"es": "Usuarios", "en": "Users"},
	"admin.users_help":          {"es": "Da de alta y gestiona los usuarios de esta instancia.", "en": "Create and manage the users of this instance."},
	"admin.users_create":        {"es": "Crear usuario", "en": "Create user"},
	"admin.users_username":      {"es": "Usuario", "en": "Username"},
	"admin.users_email":         {"es": "Email (opcional)", "en": "Email (optional)"},
	"admin.users_password":      {"es": "Contraseña", "en": "Password"},
	"admin.users_col_status":    {"es": "Estado", "en": "Status"},
	"admin.users_active":        {"es": "Activo", "en": "Active"},
	"admin.users_blocked":       {"es": "Bloqueado", "en": "Blocked"},
	"admin.users_block":         {"es": "Bloquear", "en": "Block"},
	"admin.users_unblock":       {"es": "Desbloquear", "en": "Unblock"},
	"admin.users_delete":        {"es": "Borrar", "en": "Delete"},
	"admin.users_delete_confirm": {"es": "¿Borrar este usuario y todos sus datos? Esta acción no se puede deshacer.", "en": "Delete this user and all their data? This cannot be undone."},
	"admin.users_admin_badge":   {"es": "admin", "en": "admin"},
	"admin.users_err_username":  {"es": "El nombre de usuario es obligatorio", "en": "Username is required"},
	"admin.users_err_password":  {"es": "La contraseña debe tener al menos 8 caracteres", "en": "Password must be at least 8 characters"},
	"admin.users_err_exists":    {"es": "Ese nombre de usuario ya existe", "en": "That username already exists"},
	"admin.users_created":       {"es": "Usuario creado", "en": "User created"},

	// Admin: invitations
	"admin.users_invite":        {"es": "Invitar por email", "en": "Invite by email"},
	"admin.users_invite_help":   {"es": "Envía una invitación por email; la persona completará su cuenta.", "en": "Send an email invitation; the person will complete their own account."},
	"admin.users_invite_send":   {"es": "Enviar invitación", "en": "Send invitation"},
	"admin.users_invite_sent":   {"es": "Invitación creada", "en": "Invitation created"},
	"admin.users_invite_link":   {"es": "Enlace de invitación (cópialo y compártelo):", "en": "Invitation link (copy and share it):"},
	"admin.users_err_email":     {"es": "Introduce un email válido", "en": "Enter a valid email"},
	"admin.users_err_email_exists": {"es": "Ya existe un usuario con ese email", "en": "A user with that email already exists"},
	"admin.users_pending":       {"es": "Invitaciones pendientes", "en": "Pending invitations"},
	"admin.users_no_pending":    {"es": "No hay invitaciones pendientes", "en": "No pending invitations"},
	"admin.users_revoke":        {"es": "Revocar", "en": "Revoke"},
	"admin.users_registered":    {"es": "Usuarios registrados", "en": "Registered users"},

	// Invitation acceptance page
	"invite.title":            {"es": "Completa tu cuenta", "en": "Complete your account"},
	"invite.subtitle":         {"es": "Has sido invitado a WatchLog", "en": "You've been invited to WatchLog"},
	"invite.email":            {"es": "Email", "en": "Email"},
	"invite.username":         {"es": "Nombre de usuario", "en": "Username"},
	"invite.password":         {"es": "Contraseña (opcional)", "en": "Password (optional)"},
	"invite.password_help":    {"es": "Si la dejas vacía, podrás entrar con enlace mágico por email.", "en": "If left empty, you can sign in with a magic link by email."},
	"invite.submit":           {"es": "Crear cuenta", "en": "Create account"},
	"invite.invalid":          {"es": "Invitación no válida o caducada.", "en": "Invalid or expired invitation."},
	"invite.err_username":     {"es": "El nombre de usuario es obligatorio", "en": "Username is required"},
	"invite.err_username_taken": {"es": "Ese nombre de usuario ya existe", "en": "That username already exists"},
	"invite.err_password":     {"es": "La contraseña debe tener al menos 8 caracteres", "en": "Password must be at least 8 characters"},
	"invite.email_body":       {"es": "Has sido invitado a WatchLog. Crea tu cuenta con este enlace:", "en": "You've been invited to WatchLog. Create your account with this link:"},
	"email.invite_subject":    {"es": "Te han invitado a WatchLog", "en": "You've been invited to WatchLog"},
	"settings.email":           {"es": "Email", "en": "Email"},
	"settings.email_placeholder": {"es": "tu@email.com", "en": "you@email.com"},
	"settings.email_help":      {"es": "Usado para recuperación de contraseña e inicio con enlace mágico", "en": "Used for password recovery and magic link login"},

	// Forgot password
	"forgot.title":         {"es": "Recuperar contraseña", "en": "Forgot password"},
	"forgot.description":   {"es": "Introduce tu usuario o email para recibir un enlace de restablecimiento.", "en": "Enter your username or email to receive a reset link."},
	"forgot.username_or_email": {"es": "Usuario o email", "en": "Username or email"},
	"forgot.submit":        {"es": "Enviar enlace", "en": "Send reset link"},
	"forgot.success":       {"es": "Si la cuenta existe y tiene email configurado, recibirás un enlace.", "en": "If the account exists and has an email configured, you will receive a link."},
	"forgot.smtp_disabled": {"es": "El envío de emails no está configurado. Contacta al administrador.", "en": "Email sending is not configured. Contact the administrator."},
	"forgot.back_login":    {"es": "← Volver al login", "en": "← Back to login"},

	// Reset password
	"reset.title":          {"es": "Nueva contraseña", "en": "New password"},
	"reset.new_password":   {"es": "Nueva contraseña", "en": "New password"},
	"reset.submit":         {"es": "Cambiar contraseña", "en": "Change password"},
	"reset.success":        {"es": "Contraseña actualizada. Ya puedes iniciar sesión.", "en": "Password updated. You can now log in."},
	"reset.invalid_token":  {"es": "El enlace ha expirado o no es válido.", "en": "This link has expired or is invalid."},
	"reset.password_min":   {"es": "La contraseña debe tener al menos 8 caracteres", "en": "Password must be at least 8 characters"},

	// Magic login
	"magic.title":          {"es": "Acceso con enlace mágico", "en": "Magic link login"},
	"magic.description":    {"es": "Introduce tu usuario para recibir un enlace de acceso en tu email.", "en": "Enter your username to receive a login link via email."},
	"magic.submit":         {"es": "Enviar enlace", "en": "Send magic link"},
	"magic.success":        {"es": "Si la cuenta existe y tiene email configurado, recibirás un enlace de acceso.", "en": "If the account exists and has an email configured, you'll receive a login link."},
	"magic.smtp_disabled":  {"es": "El envío de emails no está configurado. Contacta al administrador.", "en": "Email sending is not configured. Contact the administrator."},
	"magic.invalid_token":  {"es": "El enlace ha expirado o no es válido.", "en": "This link has expired or is invalid."},

	// Email subjects
	"email.reset_subject":  {"es": "WatchLog — Restablecer contraseña", "en": "WatchLog — Reset your password"},
	"email.magic_subject":  {"es": "WatchLog — Tu enlace de acceso", "en": "WatchLog — Your login link"},

	// Register email field
	"register.email":       {"es": "Email (opcional)", "en": "Email (optional)"},

	// Import
	"import.title":       {"es": "Importar datos", "en": "Import data"},
	"import.subtitle":    {"es": "Sube el archivo ZIP de exportación de TVTime", "en": "Upload your TVTime export ZIP file"},
	"history.title":            {"es": "Importar historial de visualización", "en": "Import viewing history"},
	"history.subtitle":         {"es": "Ajusta las fechas de visionado a partir de tu historial. Fuente soportada: Netflix.", "en": "Adjust watched dates from your viewing history. Supported source: Netflix."},
	"history.link":             {"es": "Importar historial de visualización", "en": "Import viewing history"},
	"history.source":           {"es": "Fuente", "en": "Source"},
	"history.select_file":      {"es": "Archivo CSV", "en": "CSV file"},
	"history.netflix_hint":     {"es": "Sube el archivo NetflixViewingHistory.csv (Cuenta → Actividad de visionado → Descargar todo).", "en": "Upload NetflixViewingHistory.csv (Account → Viewing activity → Download all)."},
	"history.analyze":          {"es": "Analizar", "en": "Analyze"},
	"history.summary":          {"es": "Resumen", "en": "Summary"},
	"history.entries":          {"es": "entradas", "en": "entries"},
	"history.changes":          {"es": "cambios", "en": "changes"},
	"history.unmatched":        {"es": "sin identificar", "en": "unmatched"},
	"history.select_all":       {"es": "Seleccionar todo", "en": "Select all"},
	"history.movie":            {"es": "Película", "en": "Movie"},
	"history.netflix_title":    {"es": "Título en Netflix", "en": "Netflix title"},
	"history.apply_selected":   {"es": "Aplicar seleccionados", "en": "Apply selected"},
	"history.backup_note":      {"es": "Se creará una copia de seguridad de la base de datos antes de aplicar.", "en": "A database backup will be created before applying."},
	"history.no_changes":       {"es": "No se encontraron fechas que corregir.", "en": "No dates to correct were found."},
	"history.try_again":        {"es": "Probar con otro archivo", "en": "Try another file"},
	"history.unmatched_series": {"es": "Series no encontradas en tu biblioteca", "en": "Series not found in your library"},
	"history.applied":          {"es": "Cambios aplicados", "en": "Changes applied"},
	"history.failed":           {"es": "Fallidos", "en": "Failed"},
	"history.backup":           {"es": "Copia de seguridad", "en": "Backup"},
	"history.back":             {"es": "Volver al perfil", "en": "Back to profile"},
	"history.error.upload":     {"es": "Error al subir el archivo.", "en": "Error uploading the file."},
	"history.error.no_file":    {"es": "No se ha seleccionado ningún archivo.", "en": "No file selected."},
	"history.error.parse":      {"es": "No se pudo analizar el archivo CSV.", "en": "Could not parse the CSV file."},
	"history.error.backup":     {"es": "No se pudo crear la copia de seguridad; no se aplicó ningún cambio.", "en": "Could not create backup; no changes applied."},
	"history.batches":          {"es": "Importaciones anteriores", "en": "Previous imports"},
	"history.review_title":     {"es": "Revisar importación", "en": "Review import"},
	"history.status_applied":   {"es": "aplicada", "en": "applied"},
	"history.status_pending":   {"es": "pendiente", "en": "pending"},
	"history.selected":         {"es": "seleccionados", "en": "selected"},
	"history.prev":             {"es": "Anteriores", "en": "Previous"},
	"history.next":             {"es": "Siguientes", "en": "Next"},
	"history.discard":          {"es": "Descartar importación", "en": "Discard import"},
	"history.confirm_delete":   {"es": "¿Descartar esta importación y sus cambios?", "en": "Discard this import and its changes?"},
	"history.all_batches":      {"es": "Todas las importaciones", "en": "All imports"},
	"history.not_found":        {"es": "No encontradas", "en": "Not found"},
	"history.episodes":         {"es": "episodios", "en": "episodes"},
	"history.search_tmdb":      {"es": "Buscar en TMDB", "en": "Search TMDB"},
	"history.select":           {"es": "Elegir", "en": "Select"},
	"history.no_candidates":    {"es": "Sin resultados en TMDB.", "en": "No TMDB results."},
	"history.tmdb_disabled":    {"es": "TMDB no está configurado; no se pueden buscar coincidencias.", "en": "TMDB is not configured; cannot search for matches."},
	"history.dates_applied":    {"es": "fechas aplicadas", "en": "dates applied"},
	"history.examples":         {"es": "Ej.", "en": "e.g."},
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
