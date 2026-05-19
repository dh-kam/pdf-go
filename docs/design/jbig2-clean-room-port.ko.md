# JBIG2 Pure Go Clean-Room Port

## 목표

`JBIG2Decode`를 `jbig2dec` C/AGPL 구현에 의존하지 않는 pure Go 구현으로 대체한다. 이 작업은 C 소스의 번역이 아니라, 공개 포맷 문서와 독립 테스트 fixture를 기준으로 한 재구현이다.

## Clean-Room 원칙

- `jbig2dec` 또는 다른 copyleft JBIG2 decoder의 구현 소스는 포팅 입력으로 사용하지 않는다.
- 허용 입력은 PDF/JBIG2 포맷 문서, 자체 생성 fixture, 공개 샘플 PDF의 관찰 가능한 입출력, 그리고 프로젝트 내부 테스트 요구사항이다.
- 외부 decoder는 결과 비교 oracle로만 사용할 수 있다. 구현 세부 알고리즘, 테이블, 코드 구조를 베껴오지 않는다.
- 새 Go 코드는 작은 단계로 추가하고, 각 단계마다 독립 fixture와 no-CGo 테스트를 남긴다.

## 구현 순서

1. 파일 헤더와 segment header parser
2. page information, end-of-page, end-of-file 처리
3. generic region segment header와 adaptive template parser
4. generic region MMR path
5. arithmetic decoder와 context template
6. generic arithmetic region
7. symbol dictionary
8. text region composition
9. refinement/halftone/extension segment
10. PDF `JBIG2Globals` DecodeParms 연결
11. cgo fallback 제거 또는 명시적 opt-in 전환

## 현재 상태

native 구현은 segment 구조, page information, generic region header, adaptive template 좌표를 읽는 기반 단계다. generic MMR region은 all-white vertical-0 line, vertical ±1, pass mode, 일부 horizontal terminating-run 조합(white 0/1/2/3/4/8, black 0/4/5/6/7/8)을 실제 `image.Gray`로 디코딩하고, page information이 있으면 region x/y에 맞춰 page-sized image로 합성한다. 그 외 bitmap-producing segment는 아직 성공으로 위장하지 않고 `NotImplemented` 또는 명시적 구조 오류를 반환한다. 실제 bitmap entropy decoding은 아직 대부분 남아 있으므로, release 빌드에서는 기존 no-CGo 테스트와 sample parity gate로 회귀를 확인해야 한다.
