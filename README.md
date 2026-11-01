# watermark

블로그 연재용 워터마크 로직 시뮬레이션 (Go, 표준 라이브러리만 사용).

| 편 | 주제 | 실행 |
|----|------|------|
| 01 | LSB 치환 | `go run ./cmd/01-lsb` (CLI)<br>`go run ./cmd/01-lsb-web` → http://localhost:8080 (웹 시각화) |
| 02 | 훼손 실험 (JPEG·Blur·축소) | 같은 서버의 http://localhost:8080/attacks.html |
| 03 | DCT 계수쌍 워터마크 + LSB 비교 | 같은 서버의 http://localhost:8080/dct.html |
| 04 | LSB vs DCT 한눈에 비교 (BER) | 같은 서버의 http://localhost:8080/compare.html |

| 05 | 워터마크 훼손 후 다수결 복구 | 같은 서버의 http://localhost:8080/recovery.html |

훼손 후 복구 페이지에서는 LSB/DCT, JPEG·Blur·축소 조건을 선택하고, 훼손 이미지 확인 → 메시지 복구 → 비트별 투표 결과를 단계별로 확인합니다. 복구 대상은 숨긴 메시지이며 이미지 화질 복원이 아닙니다. 추출 시 삽입 위치와 메시지 길이를 알고 있다고 가정합니다.

## 구조
- `internal/imageio` — PNG ↔ `*image.NRGBA` (premultiply 없이 다뤄야 투명 픽셀의 비트가 보존됨)
- `internal/lsb` — LSB 삽입/추출 (`[4바이트 길이][데이터]`, R→G→B 순서, Alpha 미사용). `EmbedSpread`는 프레임을 불투명 픽셀 전체에 순환 반복
- `internal/attack` — JPEG, Gaussian Blur, 축소 후 복원 (RGB만 대상, 입력을 불투명화)
- `internal/dct` — 8×8 블록 DCT 계수쌍 (2,1)/(1,2)의 대소 관계로 비트 삽입, 블록 전체에 반복 (semi-blind)
- `internal/vote` — 반복 삽입한 비트의 다수결 집계
- `internal/metrics` — MSE, PSNR, BER
- `internal/visualize` — 비트플레인, 차이 증폭 이미지
- `cmd/NN-주제` — 편별 시뮬레이션 진입점, 결과는 `out/NN/`

## 테스트
```
go test ./...
```
