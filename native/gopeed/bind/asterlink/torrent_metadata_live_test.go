package gopeed

import (
  "context"
  "testing"
  "time"
)

// This network integration test must confirm real, hash-matched metadata for
// BOTH user-reported magnet URIs before any repair build is accepted.
func TestRealMagnetMetadata(t *testing.T) {
  cases := []struct {
    hash string
    name string
    files int
  }{
    {"e23eadbaf23e8e9be501cfe726796166c7f790ae", "ROE-369", 2},
    {"432537477ba993225deba184dd61d65b8e3b64ed", "IPX-964C", 3},
  }
  for _,tc := range cases {
    t.Run(tc.name,func(t *testing.T){
      ctx,cancel:=context.WithTimeout(context.Background(),75*time.Second)
      defer cancel()
      encoded,err:=fetchCachedTorrent(ctx,tc.hash)
      if err!=nil { t.Fatalf("cache metadata unavailable: %v",err) }
      _,info,err:=parseTorrent(encoded)
      if err!=nil { t.Fatalf("Wenxi parser rejected real torrent metadata: %v",err) }
      if info.Hash!=tc.hash || info.Name!=tc.name || len(info.Files)!=tc.files {
        t.Fatalf("wrong metadata: hash=%q name=%q files=%d",info.Hash,info.Name,len(info.Files))
      }
      t.Logf("verified hash=%s name=%s files=%d",info.Hash,info.Name,len(info.Files))
      for _,file:=range info.Files {t.Logf("file: %s (%d bytes)",file.Path,file.Size)}
    })
  }
}
