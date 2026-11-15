// Package i18n holds every user-visible string of the configuration UI, in French, English and Spanish.
package i18n

import "strings"

// Lang is a UI language.
type Lang string

// The UI languages.
const (
	FR Lang = "fr"
	EN Lang = "en"
	ES Lang = "es"
)

// All lists the languages in the order the language selector shows them.
var All = []Lang{FR, EN, ES}

// Strings is one translation of the UI. Fields ending in "F" are fmt formats.
type Strings struct {
	LanguageName string

	OK, Cancel, Yes, No string
	QuitUnsaved         string
	OverwriteInvalid    string

	Language, Theme, ThemeAuto, ThemeLight, ThemeDark string

	Server, ServiceRunning, ServiceStopped, BackgroundRunning, Stopped string
	Start, Stop, InstallService, UninstallService                      string
	ConfigureFirewall, FirewallDone                                    string

	HTTPProxy, SOCKSProxy, Enabled, Listen, Port, Anonymous, AdvertiseAddress, AdvertiseHint string
	AnonymousWarningTitle, AnonymousWarning                                                  string

	Users, NoUsers, AddUser, ChangePassword, Delete, DeleteUserF, Name, Password, ConfirmPassword string

	Save, Saved, Unsaved, ConfigFileF string

	Error, ErrPasswordMismatch, ErrPasswordEmpty, ErrNameEmpty, ErrNameTaken, ErrInvalidPort, ElevationCancelled string
	ErrInvalidListen, ErrInvalidAdvertise, ErrSameAddress, ErrNoFirewall, ErrOther                               string
}

var translations = map[Lang]Strings{
	FR: {
		LanguageName: "Français",
		Language:     "Langue", Theme: "Thème", ThemeAuto: "Auto", ThemeLight: "Clair", ThemeDark: "Sombre",
		Server: "Serveur", ServiceRunning: "en marche (service)", ServiceStopped: "arrêté (service installé)",
		BackgroundRunning: "en marche (arrière-plan)", Stopped: "arrêté",
		Start: "Démarrer", Stop: "Arrêter", InstallService: "Installer le service",
		UninstallService:  "Désinstaller le service",
		ConfigureFirewall: "Configurer le pare-feu",
		FirewallDone: "Le pare-feu autorise maintenant les ports des proxys activés. Après un changement de port, " +
			"cliquez à nouveau sur ce bouton.",
		ErrNoFirewall: "ufw n'est pas installé : ouvrez les ports dans votre pare-feu à la main.",
		HTTPProxy:     "Proxy HTTP", SOCKSProxy: "Proxy SOCKS5", Enabled: "Activé", Listen: "Écoute", Port: "Port",
		Anonymous: "Accès anonyme", AdvertiseAddress: "Adresse publique (UDP)",
		AdvertiseHint:         "IP ou nom d'hôte de la box",
		AnonymousWarningTitle: "Accès anonyme",
		AnonymousWarning: "N'importe qui pourra utiliser ce proxy sans mot de passe. Exposé sur Internet, un proxy " +
			"ouvert est vite trouvé et détourné. Activer quand même ?",
		Users: "Utilisateurs", NoUsers: "Aucun utilisateur", AddUser: "Ajouter un utilisateur",
		ChangePassword: "Changer le mot de passe", Delete: "Supprimer", DeleteUserF: "Supprimer l'utilisateur %s ?",
		Name: "Nom", Password: "Mot de passe", ConfirmPassword: "Confirmer le mot de passe",
		Save: "Enregistrer", Saved: "Enregistré", Unsaved: "Modifications non enregistrées",
		ConfigFileF: "Configuration : %s",
		Error:       "Erreur", ErrPasswordMismatch: "Les mots de passe ne correspondent pas.",
		ErrPasswordEmpty: "Le mot de passe est vide.", ErrNameEmpty: "Le nom est vide.",
		ErrNameTaken: "Ce nom est déjà utilisé.", ErrInvalidPort: "Le port doit être un nombre entre 1 et 65535.",
		ElevationCancelled:  "Opération annulée : les droits administrateur ont été refusés.",
		ErrInvalidListen:    "L'adresse d'écoute doit être une adresse IP.",
		ErrInvalidAdvertise: "L'adresse publique doit être une adresse IP ou un nom d'hôte.",
		ErrSameAddress:      "Les proxies HTTP et SOCKS ne peuvent pas écouter sur la même adresse et le même port.",
		ErrOther:            "Configuration invalide :",
		OK:                  "OK", Cancel: "Annuler", Yes: "Oui", No: "Non",
		QuitUnsaved: "Des modifications ne sont pas enregistrées. Quitter quand même ?",
		OverwriteInvalid: "Le fichier de configuration était invalide à l'ouverture : l'enregistrer le " +
			"remplacera entièrement, avec ses utilisateurs. Continuer ?",
	},
	EN: {
		LanguageName: "English",
		Language:     "Language", Theme: "Theme", ThemeAuto: "Auto", ThemeLight: "Light", ThemeDark: "Dark",
		Server: "Server", ServiceRunning: "running (service)", ServiceStopped: "stopped (service installed)",
		BackgroundRunning: "running (background)", Stopped: "stopped",
		Start: "Start", Stop: "Stop", InstallService: "Install service", UninstallService: "Uninstall service",
		ConfigureFirewall: "Configure firewall",
		FirewallDone: "The firewall now allows the ports of the enabled proxies. Click this button again " +
			"after changing a port.",
		ErrNoFirewall: "ufw isn't installed: open the ports in your firewall by hand.",
		HTTPProxy:     "HTTP proxy", SOCKSProxy: "SOCKS5 proxy", Enabled: "Enabled", Listen: "Listen", Port: "Port",
		Anonymous: "Anonymous access", AdvertiseAddress: "Public address (UDP)",
		AdvertiseHint:         "Router IP or host name",
		AnonymousWarningTitle: "Anonymous access",
		AnonymousWarning: "Anyone will be able to use this proxy without a password. Exposed to the Internet, an " +
			"open proxy is quickly found and abused. Enable it anyway?",
		Users: "Users", NoUsers: "No users", AddUser: "Add user", ChangePassword: "Change password",
		Delete: "Delete", DeleteUserF: "Delete user %s?", Name: "Name", Password: "Password",
		ConfirmPassword: "Confirm password",
		Save:            "Save", Saved: "Saved", Unsaved: "Unsaved changes", ConfigFileF: "Configuration: %s",
		Error: "Error", ErrPasswordMismatch: "The passwords don't match.", ErrPasswordEmpty: "The password is empty.",
		ErrNameEmpty: "The name is empty.", ErrNameTaken: "This name is already used.",
		ErrInvalidPort:      "The port must be a number between 1 and 65535.",
		ElevationCancelled:  "Cancelled: administrator rights were denied.",
		ErrInvalidListen:    "The listen address must be an IP address.",
		ErrInvalidAdvertise: "The public address must be an IP address or a host name.",
		ErrSameAddress:      "The HTTP and SOCKS proxies can't listen on the same address and port.",
		ErrOther:            "Invalid configuration:",
		OK:                  "OK", Cancel: "Cancel", Yes: "Yes", No: "No",
		QuitUnsaved: "Some changes are not saved. Quit anyway?",
		OverwriteInvalid: "The configuration file was invalid when the window opened: saving " +
			"replaces it entirely, users included. Continue?",
	},
	ES: {
		LanguageName: "Español",
		Language:     "Idioma", Theme: "Tema", ThemeAuto: "Auto", ThemeLight: "Claro", ThemeDark: "Oscuro",
		Server: "Servidor", ServiceRunning: "en marcha (servicio)", ServiceStopped: "detenido (servicio instalado)",
		BackgroundRunning: "en marcha (segundo plano)", Stopped: "detenido",
		Start: "Iniciar", Stop: "Detener", InstallService: "Instalar el servicio",
		ConfigureFirewall: "Configurar el cortafuegos",
		FirewallDone: "El cortafuegos ahora permite los puertos de los proxies activados. Tras cambiar un puerto, " +
			"vuelva a pulsar este botón.",
		ErrNoFirewall:    "ufw no está instalado: abra los puertos en su cortafuegos manualmente.",
		UninstallService: "Desinstalar el servicio",
		HTTPProxy:        "Proxy HTTP", SOCKSProxy: "Proxy SOCKS5", Enabled: "Activado", Listen: "Escucha",
		Port: "Puerto", Anonymous: "Acceso anónimo", AdvertiseAddress: "Dirección pública (UDP)",
		AdvertiseHint:         "IP o nombre del router",
		AnonymousWarningTitle: "Acceso anónimo",
		AnonymousWarning: "Cualquiera podrá usar este proxy sin contraseña. Expuesto a Internet, un proxy abierto " +
			"se encuentra y se abusa rápidamente. ¿Activarlo de todos modos?",
		Users: "Usuarios", NoUsers: "Ningún usuario", AddUser: "Añadir usuario",
		ChangePassword: "Cambiar la contraseña", Delete: "Eliminar", DeleteUserF: "¿Eliminar el usuario %s?",
		Name: "Nombre", Password: "Contraseña", ConfirmPassword: "Confirmar la contraseña",
		Save: "Guardar", Saved: "Guardado", Unsaved: "Cambios sin guardar", ConfigFileF: "Configuración: %s",
		Error: "Error", ErrPasswordMismatch: "Las contraseñas no coinciden.",
		ErrPasswordEmpty: "La contraseña está vacía.", ErrNameEmpty: "El nombre está vacío.",
		ErrNameTaken: "Este nombre ya está en uso.", ErrInvalidPort: "El puerto debe ser un número entre 1 y 65535.",
		ElevationCancelled:  "Operación cancelada: se denegaron los derechos de administrador.",
		ErrInvalidListen:    "La dirección de escucha debe ser una dirección IP.",
		ErrInvalidAdvertise: "La dirección pública debe ser una dirección IP o un nombre de host.",
		ErrSameAddress:      "Los proxies HTTP y SOCKS no pueden escuchar en la misma dirección y puerto.",
		ErrOther:            "Configuración no válida:",
		OK:                  "Aceptar", Cancel: "Cancelar", Yes: "Sí", No: "No",
		QuitUnsaved: "Hay cambios sin guardar. ¿Salir de todos modos?",
		OverwriteInvalid: "El archivo de configuración no era válido al abrir la ventana: guardar lo " +
			"reemplaza por completo, usuarios incluidos. ¿Continuar?",
	},
}

// Detect maps an OS locale ("fr-FR", "es_ES"...) to a UI language, English by default.
func Detect(locale string) Lang {
	prefix, _, _ := strings.Cut(strings.ToLower(strings.ReplaceAll(locale, "_", "-")), "-")
	switch Lang(prefix) {
	case FR:
		return FR
	case ES:
		return ES
	default:
		return EN
	}
}

// For returns the strings of a language, English for an unknown one.
func For(l Lang) Strings {
	if s, ok := translations[l]; ok {
		return s
	}
	return translations[EN]
}
