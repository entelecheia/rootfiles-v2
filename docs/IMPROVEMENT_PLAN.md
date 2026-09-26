# rootfiles-v2 기능 점검 및 개선 계획

> 작성일: 2026-09-26 · 기준 커밋: `7a05ac4` · 대상: `internal/` 전체, 프로필 5종, CLI 8개 명령

`PLAN.md`는 초기 개발 계획(Phase 1–4)이고, 이 문서는 **현재 구현을 서버 bootstrap + 운영 관리 도구 관점에서 점검한 결과**와 그에 따른 개선 로드맵이다.

---

## 1. 요약

현재 도구는 "최초 1회 설치" 흐름(패키지·Docker·NVIDIA·cloudflared·사용자·GPU 할당)은 갖추었지만, **운영 서버에 반복 적용하는 관리 도구**로 쓰기에는 세 가지 구조적 공백이 있다.

1. **안전장치 부재** — SSH/UFW 설정 시 원격 접속 차단(lockout) 방지 로직이 없고, `storage` 모듈이 기존 디렉터리를 `rm -rf`로 지운다.
2. **Check ↔ Apply 불일치** — 여러 모듈에서 `Check`가 감지하는 drift를 `Apply`가 해소하지 못하거나(무한 미충족), 반대로 문서화된 플래그가 실제로는 아무 효과가 없다.
3. **운영 기능 부족** — 적용 이력/상태 저장, 기계 판독 출력(JSON), drift 모니터링용 exit code, 보안 기본선(자동 업데이트·fail2ban·시간 동기화), 사용자 수명주기(삭제·잠금·쿼터) 등이 없다.

우선순위는 **P0(안전·정확성) → P1(관리 기능) → P2(기능 확장)** 순으로 제안한다.

---

## 2. 점검 결과

### 2.1 치명적 결함 (P0 — 데이터 손실·접속 차단 위험)

| # | 위치 | 문제 | 영향 |
|---|------|------|------|
| C1 | `internal/module/storage.go:104` | 심링크 대상 경로가 일반 디렉터리면 `rm -rf link` 후 심링크 생성 | `/data`에 기존 데이터가 있으면 **무경고 삭제** |
| C2 | `internal/module/network.go:75`, `profiles/gpu-server.yaml` | `gpu-server` 프로필은 `ufw: true`인데 `allowed_ports`가 비어 있음. `--yes`로 적용 시 SSH 포트 미허용 상태로 `ufw --force enable` | **SSH lockout** |
| C3 | `internal/module/network.go` | SSH 포트를 `ssh.port`로 바꿔도 UFW에 자동 반영되지 않음 | 포트 변경 + UFW 조합 시 lockout |
| C4 | `internal/module/ssh.go:48` | `sshd -t` 검증 없이 reload. `disable_password_auth: true`인데 authorized_keys 보유 사용자가 없는지 확인 안 함 | 설정 오류·키 미등록 시 lockout |
| C5 | `internal/module/ssh.go` | Ubuntu 24.04는 `ssh.socket` 소켓 활성화 사용 → `Port` 변경은 `systemctl daemon-reload` + `ssh.socket` 재시작이 있어야 반영되는데, 현재는 `reload`만 수행 | 포트 변경이 조용히 무시됨 |
| C6 | `internal/cli/apply.go:166,232` 등 | 대화형 모드의 `ui.Confirm(..., false)`는 **프로필 값을 기본값으로 쓰지 않음**(huh 기본값 = No). dgx 프로필의 `disable_root_login: true`, `ufw: true`가 엔터만 치면 false로 바뀜 | 대화형 적용 시 보안 설정이 **조용히 약화** |
| C7 | `internal/module/users.go:680` | `sh -c "echo 'user:pass' \| chpasswd"` — 셸 인젝션 가능, 비밀번호가 프로세스 목록에 노출. 기본 비밀번호가 `username+suffix` | 보안 취약 |
| C8 | `internal/module/users.go:196` | sudoers 파일을 `visudo -c` 검증 없이 기록 | 잘못된 파일 시 sudo 전체 불능 |

### 2.2 기능 결함 (P0 — Check/Apply 불일치, 동작하지 않는 기능)

| # | 위치 | 문제 |
|---|------|------|
| B1 | `internal/module/network.go:28` | `strings.Contains(stdout, "active")`는 `"Status: inactive"`에도 참 → 비활성 UFW를 감지 못함 |
| B2 | `internal/module/network.go` | 포트 확인이 부분 문자열 매칭(`"22"`가 `"2222"`에 매칭), 포트마다 `ufw status` 재실행 |
| B3 | `internal/module/docker.go:98` | `daemon.json`을 **통째로 덮어씀** → DGX OS 기본/`nvidia-ctk`가 넣은 nvidia runtime 설정 삭제. data-root 변경 시 기존 이미지 마이그레이션 없음 |
| B4 | `internal/module/nvidia.go:42` | runtime 설정(`nvidia-ctk runtime configure`)이 "toolkit 미설치" 분기 안에만 있음 → B3 이후 Check는 영원히 미충족, Apply는 아무것도 안 함 |
| B5 | `internal/module/cloudflared.go` | `apply`가 `tunnel_token`을 사용하지 않음 → `apply --tunnel-token`은 **무효**. Check는 서비스 상태·VLAN 주소 변경을 보지 않음 |
| B6 | `internal/cli/apply.go:134` | README에 문서화된 `apply --user --ssh-pubkey`가 무시됨(`user add`에서만 사용) |
| B7 | `internal/config/loader.go:78` | `--config` 파일의 `extends`가 해석되지 않음 → 커스텀 프로필 상속 불가 |
| B8 | `internal/config/loader.go:126,175` | 병합이 "true일 때만 overlay 우선" → 자식 프로필에서 모듈/옵션을 **끌 수 없음**(bool zero-value 문제). `append(base.PackagesExtra, …)` 슬라이스 aliasing |
| B9 | `internal/module/storage.go`, `locale.go` | Apply가 변경 여부와 관계없이 `Changed: true` 보고 → 요약·감사 로그 부정확 |
| B10 | 전 모듈 | `rc.Runner.Run(...)` 반환 에러를 다수 무시(usermod, ufw allow, systemctl, locale-gen 등) → 실패해도 "✓" 출력 |
| B11 | `internal/exec/apt.go` | `DEBIAN_FRONTEND=noninteractive`, dpkg lock 대기(`DPkg::Lock::Timeout`), conffile 옵션 없음 → tzdata/conffile 프롬프트나 unattended-upgrades와의 lock 충돌로 **무인 설치 중단** |
| B12 | `internal/module/users.go:486` | `rehome`이 rsync 성공 후 검증 없이 `rm -rf oldHome` |

### 2.3 실행 인프라 공백

- **root 권한 확인 없음** — 일반 사용자로 `apply` 시 모듈마다 제각각 실패.
- **동시 실행 잠금 없음** — `apply` 두 개가 동시에 돌면 apt/설정 파일 경합 (GPU DB만 flock 보호).
- **취소/타임아웃 없음** — `context.Background()` 사용, Ctrl-C 시그널 미처리, 외부 명령 타임아웃 없음.
- **변경 전 백업 없음** — `/etc/ssh/…`, `daemon.json`, `/etc/default/useradd` 등 덮어쓰기 전 원본 보존 안 함 → 롤백 불가.
- **상태·이력 미저장** — 어떤 프로필을 언제 어떤 버전으로 적용했는지 기록 없음. `status`는 프로필을 *추측*(`SuggestProfile`)함.
- **감사 로그 없음** — slog가 stderr로만 출력, `/var/log`에 남지 않음.
- **`exec.Runner` 테스트 부재 / 인터페이스 아님** — 모듈 테스트가 실제 시스템 명령에 의존하기 쉬움.

### 2.4 관리 도구로서 누락된 기능

| 영역 | 현재 | 필요 |
|------|------|------|
| 출력 | 사람용 텍스트만 | `check`/`status`의 `--output json`, drift 시 non-zero exit code(모니터링·cron 연동) |
| 설정 | 프로필 선택만 | `config show`(병합 결과), `config validate`(unknown key 검출), `config init`(snapshot 기반 템플릿) |
| 진단 | `check` | `doctor`: 접속 경로(SSH 키/포트/UFW/tunnel), 디스크, GPU 드라이버, 재부팅 필요 여부 |
| 사용자 | add/list/backup/restore/rehome/groups/passwd | `user del`/`lock`/`unlock`, SSH 키 add/remove/list, 계정 만료, 홈 디스크 사용량, (선택) XFS project quota |
| 보안 기본선 | fail2ban 패키지만 설치 | unattended-upgrades 설정, fail2ban sshd jail, 시간 동기화(chrony/timesyncd), sysctl 기본값, SSH 추가 옵션(`AllowGroups`, `MaxAuthTries`, `KbdInteractiveAuthentication`) |
| 시스템 | locale/timezone | hostname·`/etc/hosts`, swap, journald 용량 제한, APT 미러(국내 미러), `needrestart`/reboot-required 알림 |
| GPU | toolkit + 사용자 할당 | 드라이버/CUDA 버전 점검, `nvidia-persistenced`, HGX용 `nvidia-fabricmanager` 상태, DCGM, (선택) MIG |
| 모니터링 | 없음 | node_exporter / dcgm-exporter 선택 설치, 주기적 `check` systemd timer |
| 비밀값 | token을 CLI 인자로 전달 | `--tunnel-token-file`, stdin 입력, 프로세스 인자 노출 방지 |

---

## 3. 개선 로드맵

### Phase A — 안전성·정확성 (P0, 최우선)

목표: "`--yes`로 돌려도 서버를 잃지 않는다", "Check가 말하는 것과 Apply가 하는 것이 같다".

1. **Lockout 방지 가드** (`internal/module/ssh.go`, `network.go`)
   - SSH 적용 전 `sshd -t -f` 검증, 실패 시 파일 원복.
   - `disable_password_auth`/`disable_root_login` 적용 전, sudo 가능 사용자 중 authorized_keys 보유자가 1명 이상인지 확인(없으면 에러, `--force`로만 우회).
   - UFW 활성화 시 **현재 SSH 포트를 항상 자동 허용**. `gpu-server` 프로필에 `allowed_ports: [22]` 추가.
   - `ssh.socket` 활성 여부를 감지해, 포트 변경 시 `daemon-reload` + `restart ssh.socket` 수행.
   - 서비스명 탐지(`ssh` vs `sshd`)를 명시적으로.
2. **파괴적 동작 제거** (`storage.go`, `users.go`)
   - 비어있지 않은 디렉터리는 `rm -rf` 금지 → 에러 또는 `<path>.rootfiles-bak-<ts>`로 이동.
   - `rehome`은 rsync 후 검증(`rsync --checksum --dry-run` 차이 없음) 후 원본을 백업 이름으로 이동, 삭제는 별도 옵션.
3. **대화형 기본값 버그 수정** (`internal/ui/prompt.go`, `apply.go`)
   - `ui.Confirm(msg, default, unattended)` 형태로 시그니처 변경, 프로필 값을 기본값으로. `Select`도 기본 선택 반영.
4. **모듈 Check/Apply 정합성**
   - `network`: `ufw status` 한 번 파싱해 `Status: active` 정확 비교, 포트/프로토콜 규칙 집합 비교.
   - `docker`: `daemon.json`을 JSON으로 읽어 **키 단위 병합**(data-root, log-opts만 관리), 변경 시에만 restart.
   - `nvidia`: 설치와 runtime 설정을 분리, `daemon.json`의 `runtimes.nvidia`를 JSON으로 확인.
   - `cloudflared`: `tunnel_token`이 있으면 서비스 설치/상태 확인, VLAN 주소 변경을 파일 내용 비교로 감지.
   - `storage`/`locale`: 실제 변경 시에만 `Changed: true`.
   - 전 모듈: 무시된 `Run` 에러 전파(허용 실패는 명시적 `bestEffort()` 헬퍼로 표시).
5. **설정 로더 수정** (`internal/config/`)
   - 모듈 `enabled` 및 보안 bool을 `*bool`로 변경해 자식 프로필에서 명시적 false 허용.
   - `--config` 파일도 `extends` 해석(내장 프로필 또는 상대 경로).
   - `yaml.Decoder.KnownFields(true)`로 오타 키 검출.
   - `apply --user/--ssh-pubkey`를 users 모듈에 연결(선언형 `users.accounts: []` 목록 도입 검토).
6. **APT 무인 안정성** (`internal/exec/apt.go`, `runner.go`)
   - `Runner.Run`에 env 지원 추가, apt 호출에 `DEBIAN_FRONTEND=noninteractive`, `-o DPkg::Lock::Timeout=300`, `-o Dpkg::Options::=--force-confold`.
7. **비밀번호/권한 처리** (`users.go`)
   - `chpasswd`는 stdin으로 전달(셸 미사용), 기본 비밀번호 생성은 랜덤 + 최초 로그인 시 변경 강제(`chage -d 0`).
   - sudoers는 임시 파일 → `visudo -cf` → rename.

**완료 기준**: 각 항목에 회귀 테스트 추가, `tests/scenarios/`에 lockout-guard·storage-no-delete 시나리오 추가, `apply` 2회 연속 실행 시 두 번째는 모든 모듈 "already satisfied".

### Phase B — 실행 인프라 (P0~P1)

1. **공통 preflight**: root 확인(`os.Geteuid`), 지원 OS 확인, 디스크 여유 공간, 네트워크 도달성(apt/github).
2. **전역 잠금**: `/run/rootfiles.lock` flock — `apply`/`user`/`gpu`/`tunnel` 쓰기 명령 공통.
3. **컨텍스트 취소**: `signal.NotifyContext`로 SIGINT/SIGTERM 처리, 외부 명령 기본 타임아웃.
4. **변경 전 백업**: `Runner.WriteFile`이 기존 파일을 `/var/lib/rootfiles/backups/<ts>/`에 보존 → `rootfiles rollback <ts>` 기반 마련.
5. **상태 저장**: `/var/lib/rootfiles/state.json`에 적용 프로필·버전·시각·모듈별 결과 기록. `status`는 추측 대신 이 값을 사용.
6. **감사 로그**: `/var/log/rootfiles.log`에 JSON 라인 로그(명령, 결과, 사용자, 버전). 비밀값 마스킹.
7. **테스트 용이성**: `exec.Runner`를 인터페이스(`Commander`)로 추출, fake 구현으로 모듈 단위 테스트 확대. `internal/exec`에 테스트 추가.

### Phase C — 운영 관리 기능 (P1)

1. **기계 판독 출력**: `check`/`status`에 `--output json`; `check`는 drift 존재 시 exit 2 (Nagios/cron 규약).
2. **`rootfiles config` 명령군**: `show`(병합 결과, 비밀 마스킹), `validate`, `init --from-system`(기존 snapshot 로직 재사용).
3. **`rootfiles doctor`**: 접속 경로 점검(SSH 키 보유자·포트·UFW·tunnel), reboot-required, 드라이버/커널 불일치, 디스크 사용률, 시간 동기화 상태.
4. **사용자 수명주기**: `user del [--keep-home|--archive]`, `user lock/unlock`, `user key add/rm/list`, `user expire`, `user du`(home_base 사용량), 메타데이터 DB와 시스템 계정 불일치 리포트.
5. **주기 점검**: `rootfiles check` systemd timer 설치 옵션(결과는 state.json·로그에 기록).

### Phase D — 보안·시스템 기본선 모듈 (P1~P2)

새 모듈 추가 시 `NewRegistry()`와 `defaultOrder`를 함께 갱신(`TestRegistryDefaultOrderSync`).

| 모듈(안) | 내용 | 기본 프로필 |
|----------|------|-------------|
| `system` | hostname, `/etc/hosts`, swap, journald `SystemMaxUse`, sysctl 기본값(inotify, swappiness), APT 미러 | minimal+ |
| `security` | unattended-upgrades(보안 업데이트만, 자동 재부팅 off), fail2ban sshd jail, chrony/timesyncd, SSH 추가 하드닝 옵션 | minimal+ |
| `nvidia` 확장 | persistenced 활성화, fabricmanager 상태(HGX/DGX), 드라이버·CUDA 버전 보고 | dgx, gpu-server |
| `monitoring` | node_exporter, dcgm-exporter (opt-in) | 기본 비활성 |

### Phase E — 선택 확장 (P2)

- 사용자 디스크 쿼터(XFS project quota on `/raid`).
- MIG 파티션 선언형 관리 및 GPU 할당과 연계.
- 여러 호스트 일괄 적용은 도구 범위 밖으로 두고, Ansible/`ssh host sudo rootfiles apply --config -` 패턴을 문서화(stdin config 지원만 추가).

---

## 4. 실행 순서 및 PR 단위 제안

| 순서 | PR | 범위 | 비고 |
|------|----|------|------|
| 1 | `fix(storage,users)`: 파괴적 삭제 제거 | C1, B12 | 가장 작고 위험도 높음 |
| 2 | `fix(network,ssh)`: lockout 가드 + UFW 파싱 | C2–C5, B1, B2 | gpu-server 프로필 수정 포함 |
| 3 | `fix(ui,apply)`: 대화형 기본값 = 프로필 값 | C6 | `ui.Confirm` 시그니처 변경 |
| 4 | `fix(docker,nvidia)`: daemon.json JSON 병합 | B3, B4 | DGX 회귀 시나리오 |
| 5 | `fix(exec)`: apt noninteractive + lock timeout, 에러 전파 | B10, B11 | |
| 6 | `fix(users)`: chpasswd stdin, visudo 검증 | C7, C8 | |
| 7 | `feat(config)`: `*bool` 병합, custom extends, KnownFields | B7, B8 | 프로필 YAML 호환 유지 |
| 8 | `feat(cloudflared,apply)`: token/`--user` 연결 | B5, B6 | |
| 9 | `feat(cli)`: preflight·전역 lock·signal·state·audit log | Phase B | |
| 10+ | Phase C → D → E | | 기능별 개별 PR |

각 PR은 `make test`(race) + 해당 `tests/scenarios/*.sh` 통과를 기준으로 하고, CLAUDE.md의 conventional commit 규칙을 따른다.

---

## 5. 열린 질문

1. 대화형 모드에서 보안 설정 약화(예: root login 허용)를 선택할 때 추가 확인을 받을 것인가?
2. `users.accounts` 선언형 목록을 도입해 `apply`가 사용자까지 수렴시키게 할 것인가, 아니면 명령형 `user add` 유지?
3. unattended-upgrades를 DGX에서 켤 것인가? (NVIDIA 드라이버 자동 업그레이드는 반드시 제외 필요)
4. 상태/백업 위치를 `/var/lib/rootfiles`로 할지, 기존 `<home_base>/.rootfiles`(OS 재설치 후에도 보존)에 둘지 — 재설치 복구 시나리오를 고려하면 후자에 사본을 두는 방식 권장.
