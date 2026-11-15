// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// github.go — Puerto de cmds/utils/githubstalk.js y gitclone.js (API pública
// de GitHub, sin key: 60 peticiones/hora por IP, de sobra para un bot).
package apis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// urlGitHub es variable para que las pruebas lo apunten a un httptest.Server.
var urlGitHub = "https://api.github.com"

// MaxZip es el tope del archivo que `.gitclone` baja y reenvía. Por encima,
// WhatsApp tarda minutos en subirlo y el grupo se queda esperando.
const MaxZip = 45 << 20 // 45 MB

// ErrNoEncontrado sale cuando GitHub contesta 404: el comando avisa sin drama.
var ErrNoEncontrado = errors.New("no-encontrado")

// Regex del JS: enlace https o git@, con o sin .git final.
var regexRepoGitHub = regexp.MustCompile(`^(?:https://|git@)github\.com/([^/]+)/([^/]+?)(?:\.git)?$`)

// ParseRepoURL saca (dueño, repo) de un enlace de GitHub, o "" si no es.
func ParseRepoURL(texto string) (string, string) {
	m := regexRepoGitHub.FindStringSubmatch(strings.TrimSpace(texto))
	if m == nil {
		return "", ""
	}
	return m[1], m[2]
}

// UsuarioGitHub es la ficha que enseña `.githubstalk`.
type UsuarioGitHub struct {
	Login      string
	Nombre     string
	Bio        string
	AvatarURL  string
	HTMLURL    string
	Repos      int
	Seguidores int
	Siguiente  int
	Ubicacion  string
	Web        string
	Empresa    string
	Twitter    string
	Creado     string
}

// StalkGitHub pide la ficha pública de un usuario.
func StalkGitHub(ctx context.Context, usuario string) (*UsuarioGitHub, error) {
	usuario = strings.TrimPrefix(strings.TrimSpace(usuario), "@")
	if usuario == "" || strings.ContainsAny(usuario, "/?# ") {
		return nil, fmt.Errorf("usuario-raro")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		urlGitHub+"/users/"+usuario, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := pedir(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("tiempo-agotado")
		}
		return nil, fmt.Errorf("sin-red")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNoEncontrado
	}
	cuerpo, pasado, err := leerTecho(resp.Body, 1<<20)
	if err != nil || pasado {
		return nil, fmt.Errorf("respuesta-rara")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http-%d", resp.StatusCode)
	}

	var d struct {
		Login     string `json:"login"`
		Name      string `json:"name"`
		Bio       string `json:"bio"`
		AvatarURL string `json:"avatar_url"`
		HTMLURL   string `json:"html_url"`
		Repos     int    `json:"public_repos"`
		Followers int    `json:"followers"`
		Following int    `json:"following"`
		Location  string `json:"location"`
		Blog      string `json:"blog"`
		Company   string `json:"company"`
		Twitter   string `json:"twitter_username"`
		CreatedAt string `json:"created_at"`
	}
	if json.Unmarshal(cuerpo, &d) != nil || d.Login == "" {
		return nil, fmt.Errorf("respuesta-rara")
	}
	return &UsuarioGitHub{
		Login: d.Login, Nombre: d.Name, Bio: d.Bio, AvatarURL: d.AvatarURL,
		HTMLURL: d.HTMLURL, Repos: d.Repos, Seguidores: d.Followers,
		Siguiente: d.Following, Ubicacion: d.Location, Web: d.Blog,
		Empresa: d.Company, Twitter: d.Twitter, Creado: d.CreatedAt,
	}, nil
}

// RepoGitHub es lo que lista `.gitclone` al buscar.
type RepoGitHub struct {
	Dueno       string
	Nombre      string
	Descripcion string
	HTMLURL     string
	AvatarDueno string
	Estrellas   int
	Forks       int
	Issues      int
	Visitas     int
	Creado      string
	Actualizado string
}

// repoJSON es la forma que comparten /repos/{u}/{r} y /search/repositories.
type repoJSON struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	HTMLURL     string `json:"html_url"`
	Stars       int    `json:"stargazers_count"`
	Forks       int    `json:"forks"`
	Issues      int    `json:"open_issues"`
	Watchers    int    `json:"watchers"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Owner       struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"owner"`
}

func repoDesdeJSON(d repoJSON) RepoGitHub {
	return RepoGitHub{
		Dueno: d.Owner.Login, Nombre: d.Name, Descripcion: d.Description,
		HTMLURL: d.HTMLURL, AvatarDueno: d.Owner.AvatarURL, Estrellas: d.Stars,
		Forks: d.Forks, Issues: d.Issues, Visitas: d.Watchers,
		Creado: d.CreatedAt, Actualizado: d.UpdatedAt,
	}
}

// pedirJSON pide un endpoint JSON de la API con las cabeceras que GitHub exige.
func pedirJSON(ctx context.Context, url string, destino any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := pedir(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("tiempo-agotado")
		}
		return fmt.Errorf("sin-red")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNoEncontrado
	}
	cuerpo, pasado, err := leerTecho(resp.Body, 4<<20)
	if err != nil || pasado {
		return fmt.Errorf("respuesta-rara")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http-%d", resp.StatusCode)
	}
	if json.Unmarshal(cuerpo, destino) != nil {
		return fmt.Errorf("respuesta-rara")
	}
	return nil
}

// RepoInfo trae la ficha de un repositorio concreto.
func RepoInfo(ctx context.Context, dueno, repo string) (*RepoGitHub, error) {
	var d repoJSON
	if err := pedirJSON(ctx, urlGitHub+"/repos/"+dueno+"/"+repo, &d); err != nil {
		return nil, err
	}
	if d.Name == "" {
		return nil, fmt.Errorf("respuesta-rara")
	}
	r := repoDesdeJSON(d)
	return &r, nil
}

// BuscarRepos busca texto libre y devuelve como mucho 5 resultados.
func BuscarRepos(ctx context.Context, consulta string) ([]RepoGitHub, error) {
	var d struct {
		Items []repoJSON `json:"items"`
	}
	direccion := urlGitHub + "/search/repositories?q=" + url.QueryEscape(consulta)
	if err := pedirJSON(ctx, direccion, &d); err != nil {
		return nil, err
	}
	if len(d.Items) == 0 {
		return nil, ErrNoEncontrado
	}
	if len(d.Items) > 5 {
		d.Items = d.Items[:5]
	}
	out := make([]RepoGitHub, 0, len(d.Items))
	for _, it := range d.Items {
		out = append(out, repoDesdeJSON(it))
	}
	return out, nil
}

// Zipball baja el .zip del repo. Devuelve también el nombre de archivo que
// sugiere GitHub (usuario-repo-sha.zip). Por encima de MaxZip no se baja.
func Zipball(ctx context.Context, dueno, repo string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		urlGitHub+"/repos/"+dueno+"/"+repo+"/zipball", nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := pedir(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", fmt.Errorf("tiempo-agotado")
		}
		return nil, "", fmt.Errorf("sin-red")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", ErrNoEncontrado
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("http-%d", resp.StatusCode)
	}
	datos, pasado, err := leerTecho(resp.Body, MaxZip)
	if err != nil {
		return nil, "", fmt.Errorf("sin-red")
	}
	if pasado {
		return nil, "", fmt.Errorf("muy-grande")
	}

	nombre := dueno + "-" + repo + ".zip"
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if i := strings.Index(cd, "filename="); i >= 0 {
			if n := strings.Trim(strings.TrimSpace(cd[i+9:]), `"`); n != "" {
				nombre = n
			}
		}
	}
	return datos, nombre, nil
}

// DescargarURL trae bytes de una URL ya conocida (el avatar de GitHub): plazo
// y tope cortos porque una imagen de perfil pesa poco.
func DescargarURL(ctx context.Context, url string, techo int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := pedir(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	datos, pasado, err := leerTecho(resp.Body, techo)
	if err != nil || pasado {
		return nil, fmt.Errorf("descarga-rara")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http-%d", resp.StatusCode)
	}
	return datos, nil
}
