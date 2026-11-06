#!/bin/sh
# Shin-MD-Go — preparación de direcciones de origen IPv6
#
# Deja el servidor listo para que el bot pueda salir por distintas direcciones
# de SU PROPIO bloque. No abre ningún proxy: solo acepta que el kernel use las
# direcciones del prefijo y cierra todo lo demás.
#
# Uso:
#   sh deploy/red-ipv6.sh revisar                    # solo mirar qué hay
#   sh deploy/red-ipv6.sh montar 2001:db8:1:2::/64   # aplicar (requiere sudo)
#   sh deploy/red-ipv6.sh probar   2001:db8:1:2::/64 # comprobar que sale
#   sh deploy/red-ipv6.sh deshacer 2001:db8:1:2::/64
#
# Lo que este script NO hace, a propósito: saltar captchas, generar poToken ni
# abrir puertos al mundo. Su único fin es administrar direcciones propias.

set -eu

rojo() { printf '\033[31m%s\033[0m\n' "$*"; }
verde() { printf '\033[32m%s\033[0m\n' "$*"; }
gris() { printf '\033[90m%s\033[0m\n' "$*"; }

necesita_root() {
  if [ "$(id -u)" -ne 0 ]; then
    rojo "Esto necesita root. Prueba con: sudo sh $0 $*"
    exit 1
  fi
}

# ---------------------------------------------------------------- revisar

revisar() {
  echo "== direcciones IPv6 del equipo =="
  ip -6 addr show scope global 2>/dev/null || echo "  (ninguna direccion global)"
  echo
  echo "== prefijos enrutados =="
  ip -6 route show 2>/dev/null | head -20 || echo "  (sin rutas)"
  echo
  echo "== IPv6 hacia internet =="
  if curl -6 -s -m 8 https://api64.ipify.org; then
    echo
    verde "hay salida IPv6"
  else
    rojo "no hay salida IPv6 desde aqui"
    gris "en Termux suele funcionar con datos moviles; en VPS depende del proveedor"
  fi
  echo
  echo "== soporte del kernel =="
  for clave in net.ipv6.conf.all.forwarding net.ipv6.ip_nonlocal_bind; do
    printf '  %-40s %s\n' "$clave" "$(sysctl -n "$clave" 2>/dev/null || echo 'no disponible')"
  done
}

# ---------------------------------------------------------------- montar

montar() {
  prefijo=${1:-}
  if [ -z "$prefijo" ]; then
    rojo "Falta el prefijo. Ejemplo: sh $0 montar 2001:db8:1:2::/64"
    exit 1
  fi
  necesita_root "$@"

  if ! ip -6 route show | grep -q .; then
    gris "aviso: no se ven rutas IPv6; el proveedor podria no estar enrutando el bloque"
  fi

  echo "1) kernel: aceptar direcciones no locales"
  cat > /etc/sysctl.d/99-shin-red.conf <<EOF
# Shin-MD: permitir que el bot use direcciones de su propio prefijo
net.ipv6.conf.all.forwarding = 1
net.ipv6.ip_nonlocal_bind = 1
EOF
  sysctl -q --system >/dev/null 2>&1 || true
  verde "   sysctl aplicado"

  echo "2) ruta: todo el prefijo se resuelve en local"
  ip -6 route replace local "$prefijo" dev lo
  verde "   $prefijo -> lo"

  echo "3) cortafuegos: nada abierto al mundo"
  if command -v nft >/dev/null 2>&1; then
    nft list table inet shin_red >/dev/null 2>&1 || nft add table inet shin_red
    nft flush table inet shin_red
    nft add chain inet shin_red entrada '{ type filter hook input priority 0 ; policy accept ; }'
    # Entrante nuevo hacia el prefijo: se rechaza salvo loopback y conexiones
    # ya establecidas. Asi el bloque no se convierte en proxy abierto.
    nft add rule inet shin_red entrada iif lo accept
    nft add rule inet shin_red entrada ip6 saddr "$prefijo" accept
    nft add rule inet shin_red entrada ct state established,related accept
    nft add rule inet shin_red entrada ip6 daddr "$prefijo" drop
    verde "   nftables: entrante al bloque cerrado"
  else
    gris "   nft no esta instalado; revisa el cortafuegos a mano"
  fi

  echo
  verde "listo. Comprueba con: sh $0 probar $prefijo"
}

# ---------------------------------------------------------------- probar

probar() {
  prefijo=${1:-}
  if [ -z "$prefijo" ]; then
    rojo "Falta el prefijo. Ejemplo: sh $0 probar 2001:db8:1:2::/64"
    exit 1
  fi

  base=${prefijo%%/*}
  # se toma la parte de red y se prueba con las primeras direcciones del bloque
  caso1=$(printf '%s' "$base" | sed 's/::$/::1/; s/:0*$/::1/')
  caso2=$(printf '%s' "$base" | sed 's/::$/::2/; s/:0*$/::2/')

  echo "== ¿el prefijo esta enrutado? =="
  ip -6 route show | grep -F "$prefijo" || gris "  no aparece una ruta local para $prefijo"

  echo
  echo "== salida por direcciones distintas del bloque =="
  for dir in "$caso1" "$caso2"; do
    printf '  %-32s ' "$dir"
    # --interface fuerza la direccion de origen sin tocar el resto
    if curl -6 -s -m 10 --interface "$dir" https://api64.ipify.org -o /tmp/shin_ip.$$; then
      visto=$(cat /tmp/shin_ip.$$)
      rm -f /tmp/shin_ip.$$
      if [ "$visto" = "$dir" ]; then
        verde "sale como $visto"
      else
        printf 'sale como %s\n' "$visto"
      fi
    else
      rojo "no responde"
    fi
  done
  echo
  gris "si ambas salen igual, el proveedor no enruta el bloque hacia ti:"
  gris "pidele la ruta estatica del /64 hacia tu IP, o pide un /64 on-link"
}

# ---------------------------------------------------------------- deshacer

deshacer() {
  prefijo=${1:-}
  necesita_root "$@"
  [ -n "$prefijo" ] && ip -6 route del local "$prefijo" dev lo 2>/dev/null || true
  rm -f /etc/sysctl.d/99-shin-red.conf
  if command -v nft >/dev/null 2>&1; then
    nft delete table inet shin_red 2>/dev/null || true
  fi
  sysctl -q -w net.ipv6.ip_nonlocal_bind=0 2>/dev/null || true
  verde "deshecho"
}

case ${1:-revisar} in
  revisar) revisar ;;
  montar) shift; montar "$@" ;;
  probar) shift; probar "$@" ;;
  deshacer) shift; deshacer "$@" ;;
  *)
    echo "uso: sh $0 {revisar|montar <prefijo>|probar <prefijo>|deshacer <prefijo>}"
    exit 1
    ;;
esac
