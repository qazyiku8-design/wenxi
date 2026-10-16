package gopeed

import (
  "context"
  "encoding/base64"
  "io"
  "net/http"
  "strings"
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
      if err!=nil {
        for _,base:=range []string{"https://itorrents.org/torrent/","https://btcache.me/torrent/","https://torrentcache.top/torrent/"} {
          request,_:=http.NewRequestWithContext(ctx,"GET",base+strings.ToUpper(tc.hash)+".torrent",nil)
          response,err:=(&http.Client{Timeout:8*time.Second}).Do(request)
          if err!=nil {t.Logf("cache %s error %v",base,err);continue}
          raw,_:=io.ReadAll(io.LimitReader(response.Body,4*1024*1024+1));response.Body.Close()
          _,meta,parseErr:=parseTorrent(torrentPrefix+base64.StdEncoding.EncodeToString(raw))
          if parseErr!=nil {t.Logf("cache %s status=%d len=%d parse=%v",base,response.StatusCode,len(raw),parseErr)} else {t.Logf("cache %s status=%d name=%s hash=%s",base,response.StatusCode,meta.Name,meta.Hash)}
        }
        t.Fatalf("cache metadata unavailable: %v",err)
      }
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
