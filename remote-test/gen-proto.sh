# Regenerate pb code after proto changes (run on dev machine; protoc +
# go plugins live in ~/tools/protoc/bin + GOBIN).
#   node gen-proto.mjs   (or: bash - <<'EOF' ... )
# We keep it as a shell script run through ssh.js-less local bash; the .pb.go
# outputs land in backend/pb/.
set -e
cd "$(dirname "$0")/../backend/proto"
export PATH="$PATH:/c/Users/Asanagi/tools/protoc/bin:/c/Users/Asanagi/tools/go-sdk/go/bin"
protoc --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative oj.proto
mv oj.pb.go oj_grpc.pb.go ../pb/
echo "pb regenerated"
