#!/bin/bash

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname "$0")" && pwd)"
PACKAGE_ROOT="$(CDPATH= cd -- "${SCRIPT_DIR}/.." 2>/dev/null && pwd)"

APPDEST_DIR="${TRIM_APPDEST:-}"
PKG_ETC_DIR="${TRIM_PKGETC:-}"
PKG_VAR_DIR="${TRIM_PKGVAR:-}"

if [ -z "${APPDEST_DIR}" ] && [ -n "${PACKAGE_ROOT}" ]; then
    if [ -L "${PACKAGE_ROOT}/target" ] || [ -d "${PACKAGE_ROOT}/target" ]; then
        APPDEST_DIR="$(CDPATH= cd -- "${PACKAGE_ROOT}/target" 2>/dev/null && pwd)"
    elif [ -d "${PACKAGE_ROOT}/app" ]; then
        APPDEST_DIR="$(CDPATH= cd -- "${PACKAGE_ROOT}/app" 2>/dev/null && pwd)"
    fi
fi

if [ -z "${PKG_ETC_DIR}" ] && [ -n "${PACKAGE_ROOT}" ]; then
    if [ -L "${PACKAGE_ROOT}/etc" ] || [ -d "${PACKAGE_ROOT}/etc" ]; then
        PKG_ETC_DIR="$(CDPATH= cd -- "${PACKAGE_ROOT}/etc" 2>/dev/null && pwd)"
    else
        PKG_ETC_DIR="${PACKAGE_ROOT}/etc"
    fi
fi

if [ -z "${PKG_VAR_DIR}" ] && [ -n "${PACKAGE_ROOT}" ]; then
    if [ -L "${PACKAGE_ROOT}/var" ] || [ -d "${PACKAGE_ROOT}/var" ]; then
        PKG_VAR_DIR="$(CDPATH= cd -- "${PACKAGE_ROOT}/var" 2>/dev/null && pwd)"
    else
        PKG_VAR_DIR="${PACKAGE_ROOT}/var"
    fi
fi

APP_BIN="${APPDEST_DIR}/server/superflare"
DEFAULTS_DIR="${APPDEST_DIR}/server/defaults"
ETC_DIR="${PKG_ETC_DIR}"
VAR_DIR="${PKG_VAR_DIR}"
LEGACY_RUNTIME_DIR="${VAR_DIR}/runtime"
WORK_DIR="${ETC_DIR}"
RUN_DIR="${VAR_DIR}/run"
PID_FILE="${RUN_DIR}/superflare.pid"
APP_LOG="${RUN_DIR}/superflare.log"
CONFIG_LOCK_FILE="${ETC_DIR}/.superflare-config.lock"
TMP_LOG_FILE="${TRIM_TEMP_LOGFILE:-}"
if [ -z "${TMP_LOG_FILE}" ]; then
    if [ -n "${RUN_DIR}" ]; then
        TMP_LOG_FILE="${RUN_DIR}/lifecycle.log"
    elif [ -n "${PACKAGE_ROOT}" ]; then
        TMP_LOG_FILE="${PACKAGE_ROOT}/lifecycle.log"
    else
        TMP_LOG_FILE="/tmp/superflare-lifecycle.log"
    fi
fi
CONFIG_FILE="${ETC_DIR}/config.yml"

timestamp() {
    date '+%Y-%m-%d %H:%M:%S'
}

log_info() {
    if [ -n "${APP_LOG}" ] && mkdir -p "$(dirname "${APP_LOG}")" 2>/dev/null; then
        printf '%s %s\n' "$(timestamp)" "$*" >> "${APP_LOG}" 2>/dev/null || true
    fi
}

log_lifecycle() {
    if [ -n "${APP_LOG}" ] && mkdir -p "$(dirname "${APP_LOG}")" 2>/dev/null; then
        printf '%s %s\n' "$(timestamp)" "$*" >> "${APP_LOG}" 2>/dev/null || true
    fi
    if [ -n "${TMP_LOG_FILE}" ] && mkdir -p "$(dirname "${TMP_LOG_FILE}")" 2>/dev/null; then
        printf '%s\n' "$*" >> "${TMP_LOG_FILE}" 2>/dev/null || true
    fi
}

ensure_dir() {
    if [ -z "${1:-}" ]; then
        return 1
    fi
    mkdir -p "$1"
}

require_path() {
    local path="$1"
    local name="$2"

    if [ -n "${path}" ]; then
        return 0
    fi

    log_lifecycle "Missing required path: ${name}"
    return 1
}

generate_secret() {
    if [ -r /dev/urandom ] && command -v tr >/dev/null 2>&1; then
        tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 48
        return 0
    fi
    date '+%s%N'
}

copy_default_file() {
    local name="$1"
    local src="${DEFAULTS_DIR}/${name}"
    local dst="${ETC_DIR}/${name}"

    if [ ! -f "${dst}" ] && [ -f "${src}" ]; then
        cp "${src}" "${dst}"
    fi
}

overwrite_default_file() {
    local name="$1"
    local src="${DEFAULTS_DIR}/${name}"
    local dst="${ETC_DIR}/${name}"

    if [ -f "${src}" ]; then
        cp "${src}" "${dst}"
    fi
}

with_config_lock() {
    if [ "$#" -eq 0 ]; then
        return 1
    fi
    ensure_dir "${ETC_DIR}" || return 1
    : > "${CONFIG_LOCK_FILE}" || return 1
    if command -v flock >/dev/null 2>&1; then
        (
            flock -x 9
            "$@"
        ) 9>"${CONFIG_LOCK_FILE}"
        return $?
    fi

    "$@"
}

upsert_env_value() {
    local file="$1"
    local key="$2"
    local value="$3"
    local tmp
    local formatted
    local updated=0
    local line
    local normalized

    ensure_dir "$(dirname "${file}")" || return 1
    [ -f "${file}" ] || : > "${file}"
    formatted="$(format_env_value "${value}")"
    tmp="$(mktemp)"
    while IFS= read -r line || [ -n "${line}" ]; do
        normalized="${line#$'\xef\xbb\xbf'}"
        if [[ "${normalized}" =~ ^([[:space:]]*)(export[[:space:]]+)?${key}[[:space:]]*= ]]; then
            printf '%s%s%s=%s\n' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${key}" "${formatted}" >> "${tmp}"
            updated=1
            continue
        fi
        printf '%s\n' "${line}" >> "${tmp}"
    done < "${file}"
    if [ "${updated}" -eq 0 ]; then
        printf '%s=%s\n' "${key}" "${formatted}" >> "${tmp}"
    fi
    cat "${tmp}" > "${file}"
    rm -f "${tmp}"
}

ensure_env_key() {
    local file="$1"
    local key="$2"
    local value="$3"

    if [ -z "$(read_env_value "${file}" "${key}")" ]; then
        upsert_env_value "${file}" "${key}" "${value}"
    fi
}

read_env_value() {
    local file="$1"
    local key="$2"
    local line
    local value

    if [ ! -r "${file}" ]; then
        return 0
    fi

    while IFS= read -r line || [ -n "${line}" ]; do
        line="${line#$'\xef\xbb\xbf'}"
        if [[ "${line}" =~ ^[[:space:]]*(export[[:space:]]+)?${key}[[:space:]]*= ]]; then
            value="${line#*=}"
            parse_shell_value "${value}"
            return 0
        fi
    done < "${file}"
}

yaml_escape() {
    printf '%s' "$1" | sed "s/'/''/g"
}

trim_value() {
    printf '%s' "$1" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//'
}

format_env_value() {
    local value="$1"
    local escaped

    if [ -z "${value}" ]; then
        printf ''
        return 0
    fi

    case "${value}" in
        *[[:space:]#\;\"\'\\]*)
            escaped="${value//\\/\\\\}"
            escaped="${escaped//\"/\\\"}"
            printf '"%s"' "${escaped}"
            ;;
        *)
            printf '%s' "${value}"
            ;;
    esac
}

parse_shell_value() {
    local value
    local quote
    local rest

    value="$(trim_value "$1")"
    quote="${value:0:1}"
    if [ "${quote}" = "'" ] || [ "${quote}" = '"' ]; then
        rest="${value:1}"
        value="${rest%%${quote}*}"
        if [ "${quote}" = '"' ]; then
            value="${value//\\\"/\"}"
            value="${value//\\\\/\\}"
        else
            value="${value//\'\'/\'}"
        fi
        printf '%s\n' "${value}"
        return 0
    fi

    value="$(printf '%s' "${value}" | sed 's/[[:space:]]#.*$//;s/[[:space:]]*$//')"
    printf '%s\n' "${value}"
}

read_yaml_value() {
    local file="$1"
    local key="$2"
    local line
    local value

    if [ ! -r "${file}" ]; then
        return 0
    fi

    while IFS= read -r line || [ -n "${line}" ]; do
        line="${line#$'\xef\xbb\xbf'}"
        if [[ "${line}" =~ ^[[:space:]]*${key}[[:space:]]*: ]]; then
            value="${line#*:}"
            parse_shell_value "${value}"
            return 0
        fi
    done < "${file}"
}

upsert_yaml_value() {
    local file="$1"
    local key="$2"
    local value="$3"
    local tmp
    local escaped

    escaped="$(yaml_escape "${value}")"
    tmp="$(mktemp)"
    awk -v key="${key}" -v value="${escaped}" '
        BEGIN { updated = 0 }
        index($0, key ":") == 1 {
            print key ": '\''" value "'\''"
            updated = 1
            next
        }
        { print }
        END {
            if (!updated) {
                print key ": '\''" value "'\''"
            }
        }
    ' "${file}" > "${tmp}"
    cat "${tmp}" > "${file}"
    rm -f "${tmp}"
}

ensure_env_file() {
    local env_file="${ETC_DIR}/.env"
    local secret

    if [ ! -f "${env_file}" ]; then
        cat > "${env_file}" <<EOF
FLARE_DISABLE_LOGIN=false
FLARE_EDITOR=true
FLARE_GUIDE=true
FLARE_COOKIE_NAME=superflare
FLARE_COOKIE_SECRET=$(generate_secret)
EOF
    fi

    upsert_env_value "${env_file}" "FLARE_PORT" "${TRIM_SERVICE_PORT:-3636}"
    ensure_env_key "${env_file}" "FLARE_DISABLE_LOGIN" "false"
    ensure_env_key "${env_file}" "FLARE_EDITOR" "true"
    ensure_env_key "${env_file}" "FLARE_GUIDE" "true"
    ensure_env_key "${env_file}" "FLARE_COOKIE_NAME" "superflare"

    secret="$(awk -F= '/^FLARE_COOKIE_SECRET=/{print $2; exit}' "${env_file}")"
    if [ -z "${secret}" ]; then
        upsert_env_value "${env_file}" "FLARE_COOKIE_SECRET" "$(generate_secret)"
    fi
}

ensure_login_env_defaults() {
    local env_file="${ETC_DIR}/.env"
    local user=""
    local pass=""

    user="$(read_env_value "${env_file}" "FLARE_USER")"
    pass="$(read_env_value "${env_file}" "FLARE_PASS")"

    if [ -z "${user}" ]; then
        upsert_env_value "${env_file}" "FLARE_USER" "admin"
    fi
    if [ -z "${pass}" ]; then
        upsert_env_value "${env_file}" "FLARE_PASS" "admin"
    fi
}

ensure_login_config_defaults() {
    local user=""
    local pass=""

    user="$(read_yaml_value "${CONFIG_FILE}" "LoginUser")"
    pass="$(read_yaml_value "${CONFIG_FILE}" "LoginPass")"

    if [ -z "${user}" ]; then
        upsert_yaml_value "${CONFIG_FILE}" "LoginUser" "admin"
    fi
    if [ -z "${pass}" ]; then
        upsert_yaml_value "${CONFIG_FILE}" "LoginPass" "admin"
    fi
}

sync_login_config_locked() {
    local user="$1"
    local pass="$2"

    upsert_env_value "${ETC_DIR}/.env" "FLARE_USER" "${user}"
    upsert_env_value "${ETC_DIR}/.env" "FLARE_PASS" "${pass}"
    upsert_yaml_value "${CONFIG_FILE}" "LoginUser" "${user}"
    upsert_yaml_value "${CONFIG_FILE}" "LoginPass" "${pass}"
}

sync_login_config() {
    with_config_lock sync_login_config_locked "$@"
}

sync_login_enabled_config_locked() {
    local enabled="$1"
    local disable_login="false"

    case "${enabled}" in
        true|1|yes|on|enable|enabled)
            disable_login="false"
            ;;
        false|0|no|off|disable|disabled)
            disable_login="true"
            ;;
        *)
            return 1
            ;;
    esac

    upsert_env_value "${ETC_DIR}/.env" "FLARE_DISABLE_LOGIN" "${disable_login}"
}

sync_login_enabled_config() {
    with_config_lock sync_login_enabled_config_locked "$@"
}

migrate_legacy_runtime_file() {
    local name="$1"
    local runtime_path="${LEGACY_RUNTIME_DIR}/${name}"
    local etc_path="${ETC_DIR}/${name}"

    if [ -e "${runtime_path}" ] && [ ! -L "${runtime_path}" ] && [ ! -e "${etc_path}" ]; then
        mv "${runtime_path}" "${etc_path}"
    fi
}

cleanup_legacy_runtime_layout() {
    local runtime_dir="${LEGACY_RUNTIME_DIR}"

    if [ ! -d "${runtime_dir}" ] && [ ! -L "${runtime_dir}" ]; then
        return 0
    fi

    rm -f "${runtime_dir}/.env"
    rm -f "${runtime_dir}/config.yml"
    rm -f "${runtime_dir}/apps.yml"
    rm -f "${runtime_dir}/bookmarks.yml"
    rm -f "${runtime_dir}/ports.yaml"
    rm -f "${runtime_dir}/var"

    if [ -d "${runtime_dir}" ] && [ -z "$(ls -A "${runtime_dir}" 2>/dev/null)" ]; then
        rmdir "${runtime_dir}" 2>/dev/null || true
    fi
}

relink_path() {
    local link_path="$1"
    local target_path="$2"
    local current=""

    if [ -L "${link_path}" ]; then
        current="$(readlink "${link_path}" 2>/dev/null || true)"
        if [ "${current}" = "${target_path}" ]; then
            return 0
        fi
    fi

    if [ -e "${link_path}" ] || [ -L "${link_path}" ]; then
        rm -rf "${link_path}"
    fi

    ln -s "${target_path}" "${link_path}"
}

ensure_runtime_layout_locked() {
    ensure_dir "${ETC_DIR}" &&
    ensure_dir "${VAR_DIR}" &&
    ensure_dir "${RUN_DIR}" &&
    ensure_dir "${VAR_DIR}/cache" &&
    ensure_dir "${VAR_DIR}/cache/backgrounds" &&
    ensure_dir "${VAR_DIR}/cache/site-icons" &&
    ensure_dir "${VAR_DIR}/uploads" &&
    migrate_legacy_runtime_file ".env" &&
    migrate_legacy_runtime_file "config.yml" &&
    migrate_legacy_runtime_file "apps.yml" &&
    migrate_legacy_runtime_file "bookmarks.yml" &&
    migrate_legacy_runtime_file "ports.yaml" &&
    copy_default_file "config.yml" &&
    copy_default_file "apps.yml" &&
    copy_default_file "bookmarks.yml" &&
    copy_default_file "ports.yaml" &&
    ensure_env_file &&
    ensure_login_env_defaults &&
    ensure_login_config_defaults &&
    relink_path "${ETC_DIR}/var" "${VAR_DIR}" &&
    cleanup_legacy_runtime_layout
}

ensure_runtime_layout() {
    require_path "${APPDEST_DIR}" "APPDEST_DIR" || return 1
    require_path "${DEFAULTS_DIR}" "DEFAULTS_DIR" || return 1
    require_path "${ETC_DIR}" "ETC_DIR" || return 1
    require_path "${VAR_DIR}" "VAR_DIR" || return 1

    with_config_lock ensure_runtime_layout_locked
}

reset_runtime_defaults_for_install_locked() {
    ensure_dir "${ETC_DIR}" &&
    overwrite_default_file "config.yml" &&
    overwrite_default_file "apps.yml" &&
    overwrite_default_file "bookmarks.yml" &&
    overwrite_default_file "ports.yaml" || return 1

    if [ -f "${ETC_DIR}/.env" ]; then
        rm -f "${ETC_DIR}/.env" || return 1
    fi

    ensure_env_file
}

reset_runtime_defaults_for_install() {
    with_config_lock reset_runtime_defaults_for_install_locked
}

read_pid_file() {
    if [ ! -r "${PID_FILE}" ]; then
        return 1
    fi
    head -n 1 "${PID_FILE}" | tr -d '[:space:]'
}

process_matches_pid() {
    local pid="$1"
    local args=""
    local exe=""

    if [ -z "${pid}" ]; then
        return 1
    fi
    case "${pid}" in
        *[!0-9]*)
            return 1
            ;;
    esac

    if ! kill -0 "${pid}" 2>/dev/null; then
        return 1
    fi

    if [ -n "${APP_BIN}" ] && [ -r "/proc/${pid}/exe" ]; then
        exe="$(readlink "/proc/${pid}/exe" 2>/dev/null || true)"
        if [ "${exe}" = "${APP_BIN}" ]; then
            return 0
        fi
    fi

    args="$(ps -p "${pid}" -o args= 2>/dev/null || true)"
    if [ -z "${args}" ] && [ -r "/proc/${pid}/cmdline" ]; then
        args="$(tr '\0' ' ' < "/proc/${pid}/cmdline" 2>/dev/null || true)"
    fi
    if [ -z "${args}" ]; then
        return 1
    fi
    if [ -n "${APP_BIN}" ] && printf '%s\n' "${args}" | grep -F -- "${APP_BIN}" >/dev/null 2>&1; then
        return 0
    fi

    return 1
}

discover_running_pid() {
    local pid=""

    pid="$(read_pid_file 2>/dev/null || true)"
    if process_matches_pid "${pid}"; then
        printf '%s' "${pid}"
        return 0
    fi

    if command -v pgrep >/dev/null 2>&1; then
        for pid in $(pgrep -f "${APP_BIN}" 2>/dev/null || true); do
            if process_matches_pid "${pid}"; then
                printf '%s' "${pid}" > "${PID_FILE}"
                printf '%s' "${pid}"
                return 0
            fi
        done
    fi

    rm -f "${PID_FILE}"
    return 1
}

status_app() {
    discover_running_pid >/dev/null 2>&1
}

start_app() {
    local pid=""

    ensure_runtime_layout || {
        log_lifecycle "Failed to prepare SuperFlare runtime layout."
        return 1
    }

    if [ ! -x "${APP_BIN}" ]; then
        log_lifecycle "SuperFlare binary not found or not executable: ${APP_BIN}"
        return 1
    fi

    if status_app; then
        return 0
    fi

    (
        cd "${WORK_DIR}" || exit 1
        export HOME="${TRIM_PKGHOME:-${VAR_DIR}}"
        export XDG_CACHE_HOME="${VAR_DIR}/cache"
        "${APP_BIN}" --port "${TRIM_SERVICE_PORT:-3636}" >> "${APP_LOG}" 2>&1 &
        printf '%s' "$!" > "${PID_FILE}"
    )

    sleep 2
    pid="$(discover_running_pid 2>/dev/null || true)"
    if [ -n "${pid}" ]; then
        log_info "SuperFlare started with pid ${pid}"
        return 0
    fi

    log_lifecycle "SuperFlare failed to start. See ${APP_LOG}."
    if [ -f "${APP_LOG}" ]; then
        tail -n 60 "${APP_LOG}" >> "${TMP_LOG_FILE}"
    fi
    return 1
}

stop_app() {
    local pid=""
    local count=0

    pid="$(discover_running_pid 2>/dev/null || true)"
    if [ -z "${pid}" ]; then
        rm -f "${PID_FILE}"
        return 0
    fi

    kill -TERM "${pid}" 2>/dev/null || true
    while process_matches_pid "${pid}" && [ "${count}" -lt 15 ]; do
        sleep 1
        count=$((count + 1))
    done

    if process_matches_pid "${pid}"; then
        kill -KILL "${pid}" 2>/dev/null || true
        sleep 1
    fi

    rm -f "${PID_FILE}"
    log_info "SuperFlare stopped"
    return 0
}
