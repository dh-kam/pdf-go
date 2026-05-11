# 릴리즈 준비 문서 (`v0.9.0-202602.1`)

## 목적

- 현재 포팅 완료 게이트(`make porting-complete-plus-goal98`)를 통과한 상태를 기준으로
  배포 작업(Tag/GitHub Release/Go module publish)을 일관되게 수행한다.

## 사전 조건

- 저장소 루트에 `.git` 메타데이터가 존재해야 한다.
- 원격 저장소 push 권한이 있어야 한다.
- `GITHUB_TOKEN` 또는 릴리즈 생성 권한이 준비되어야 한다.

## 릴리즈 전 검증

```bash
make porting-complete-plus-goal98
```

검증 결과 확인 포인트:

- `check-no-cgo` 통과
- core coverage gate `>= 80%` 통과
- goal98 diff 회귀 없음(`delta=0`)

자동화 preflight(권장):

```bash
make release-preflight RELEASE_VERSION=v0.9.0-202602.1
```

## 1) Tag 생성

```bash
git tag -a v0.9.0-202602.1 -m "release: v0.9.0-202602.1"
git push origin v0.9.0-202602.1
```

## 2) GitHub Release 생성

권장 릴리즈 노트 항목:

- 렌더 회귀 비교 자동화(HTML: Poppler/Ours/XOR + similarity PASS/FAIL)
- fail-doc 장시간 재비교 자동화
- nightly diff 게이트 추가
- coverage core gate 도입(`coverage-core-no-cgo`)
- 이미지 디코더 성능 최적화(`BenchmarkPDFRender` 개선)
- 공개 심볼 godoc 누락 `0` 달성

## 3) Go Module 퍼블리시

Go proxy 반영 확인:

```bash
GONOSUMDB=github.com/dh-kam/pdf-go go list -m github.com/dh-kam/pdf-go/pkg/pdf@v0.9.0-202602.1
```

필요 시:

```bash
GOPROXY=https://proxy.golang.org go list -m github.com/dh-kam/pdf-go/pkg/pdf@v0.9.0-202602.1
```

## 자동화 릴리즈 실행

사전 검증 + 태그/릴리즈/모듈 확인을 하나의 흐름으로 실행:

```bash
make release-publish RELEASE_VERSION=v0.9.0-202602.1 RELEASE_MODULE=github.com/dh-kam/pdf-go/pkg/pdf
```

부작용 없이 명령만 검증하려면:

```bash
make release-dry-run RELEASE_VERSION=v0.9.0-202602.1 RELEASE_DRY_RUN=true
```

## 릴리즈 후 체크리스트

- GitHub Release 아티팩트/노트 확인
- 모듈 버전 조회 확인
- `TODO.md` Distribution 섹션 완료 처리
