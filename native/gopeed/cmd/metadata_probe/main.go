package main

import (
 "context"
 "encoding/hex"
 "fmt"
 "io"
 "net/http"
 "os"
 "strings"
 "time"

 "github.com/anacrolix/torrent"
 "github.com/anacrolix/torrent/metainfo"
)

var hashes=[]string{
 "e23eadbaf23e8e9be501cfe726796166c7f790ae",
 "432537477ba993225deba184dd61d65b8e3b64ed",
}
var caches=[]string{
 "https://itorrents.org/torrent/",
 "https://btcache.me/torrent/",
 "https://torrentcache.top/torrent/",
}
func verify(raw io.Reader, want string) bool {
 mi,e:=metainfo.Load(io.LimitReader(raw,4*1024*1024+1));if e!=nil{return false}
 if !strings.EqualFold(mi.HashInfoBytes().String(),want){return false}
 info,e:=mi.UnmarshalInfo(); if e!=nil{return false}
 files:=info.UpvertedFiles()
 if len(files)==0{return false}
 fmt.Printf("PASS hash=%s name=%q files=%d\\n",want,info.BestName(),len(files))
 for _,f:=range files {fmt.Printf("  %s (%d bytes)\\n",strings.Join(f.BestPath(), "/"),f.Length)}
 return true
}
func cache(ctx context.Context,hash string) bool {
 client:=&http.Client{Timeout:15*time.Second}
 for _,base:=range caches {
  req,e:=http.NewRequestWithContext(ctx,"GET",base+strings.ToUpper(hash)+".torrent",nil);if e!=nil{continue}
  resp,e:=client.Do(req);if e!=nil{fmt.Printf("CACHE error %s: %v\\n",base,e);continue}
  if resp.StatusCode==200 && verify(resp.Body,hash){resp.Body.Close();return true}
  fmt.Printf("CACHE %s HTTP %d\\n",base,resp.StatusCode)
  resp.Body.Close()
 }
 return false
}
func swarm(ctx context.Context,hash string) bool {
 cfg:=torrent.NewDefaultClientConfig()
 cfg.DataDir=os.TempDir()
 cfg.ListenPort=0
 cfg.Seed=false
 cfg.NoUpload=true
 cfg.DisableWebtorrent=true
 cfg.DisableWebseeds=true
 cfg.EstablishedConnsPerTorrent=48
 c,e:=torrent.NewClient(cfg)
 if e!=nil{fmt.Printf("client error: %v\\n",e);return false}
 defer c.Close()
 _,e=hex.DecodeString(hash);if e!=nil{return false}
 magnet:="magnet:?xt=urn:btih:"+hash
 t,e:=c.AddMagnet(magnet)
 if e!=nil{fmt.Printf("magnet error %v\\n",e);return false}
 t.AddTrackers([][]string{
  {"udp://tracker.opentrackr.org:1337/announce"},
  {"udp://tracker.torrent.eu.org:451/announce"},
  {"https://tracker.gbitt.info/announce"},
 })
 select {
 case <-t.GotInfo():
   mi:=t.Metainfo()
   info,e:=mi.UnmarshalInfo()
   if e!=nil{return false}
   if !strings.EqualFold(mi.HashInfoBytes().String(),hash){return false}
   files:=info.UpvertedFiles()
   if len(files)==0{return false}
   fmt.Printf("PASS DHT/TRACKER hash=%s name=%q files=%d\\n",hash,info.BestName(),len(files))
   for _,f:=range files {fmt.Printf("  %s (%d bytes)\\n",strings.Join(f.BestPath(), "/"),f.Length)}
   return true
 case <-ctx.Done():
   fmt.Printf("SWARM timeout hash=%s peers=%d err=%v\\n",hash,len(t.PeerConns()),ctx.Err())
   return false
 }
}
func main(){
 failures:=0
 for _,hash:=range hashes{
  fmt.Printf("START %s\\n",hash)
  ctx,cancel:=context.WithTimeout(context.Background(),150*time.Second)
  passed:=cache(ctx,hash)
  if !passed{passed=swarm(ctx,hash)}
  cancel()
  if !passed{failures++;fmt.Printf("FAIL hash=%s\\n",hash)}
 }
 if failures>0{fmt.Fprintf(os.Stderr,"%d magnet(s) could not be resolved\\n",failures);os.Exit(1)}
}
