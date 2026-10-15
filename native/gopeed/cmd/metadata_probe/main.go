package main

import (
 "context"
 "encoding/hex"
 "bytes"
 "fmt"
 "io"
 "net/http"
 "os"
 "strings"
 "time"

 "github.com/anacrolix/torrent"
 "github.com/anacrolix/torrent/metainfo"
 "github.com/anacrolix/torrent/storage"
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
  if resp.StatusCode==200 && verify(resp.Body,hash){fmt.Printf("CACHE HIT %s\\n",base);resp.Body.Close();return true}
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
func fetchTorrentBytes(ctx context.Context,hash string)([]byte,error){
 client:=&http.Client{Timeout:8*time.Second}
 for _,base:=range caches{
  req,e:=http.NewRequestWithContext(ctx,"GET",base+strings.ToUpper(hash)+".torrent",nil)
  if e!=nil{continue}
  response,e:=client.Do(req);if e!=nil{continue}
  if response.StatusCode!=200{response.Body.Close();continue}
  data,e:=io.ReadAll(io.LimitReader(response.Body,4*1024*1024+1))
  response.Body.Close()
  if e!=nil{continue}
  if !verify(bytes.NewReader(data),hash){continue}
  return data,nil
 }
 return nil,fmt.Errorf("metadata missing: %s",hash)
}
func streamFirstPiece(ctx context.Context,hash string) bool {
 encoded,er:=fetchTorrentBytes(ctx,hash)
 if er!=nil {fmt.Printf("STREAM CACHE FAIL: %v\\n",er);return false}
 cfg:=torrent.NewDefaultClientConfig()
 cfg.DataDir=os.TempDir()
 cfg.ListenPort=0
 cfg.Seed=false
 cfg.NoUpload=true
 cfg.DisableWebtorrent=true
 dir,e:=os.MkdirTemp("","wenxi-stream-probe-")
 if e!=nil {fmt.Printf("TEMP ERROR: %v\\n",e);return false}
 defer os.RemoveAll(dir)
 cfg.DataDir=dir
 cfg.DefaultStorage=storage.NewFileWithCompletion(dir,storage.NewMapPieceCompletion())
 client,e:=torrent.NewClient(cfg)
 if e!=nil {fmt.Printf("BT ERROR: %v\\n",e);return false}
 defer client.Close()
 mi,e:=metainfo.Load(bytes.NewReader(encoded))
 if e!=nil {fmt.Printf("STREAM META ERROR: %v\\n",e);return false}
 t,e:=client.AddTorrent(mi)
 if e!=nil {fmt.Printf("BT ADD ERROR: %v\\n",e);return false}
 t.AddTrackers([][]string{{"udp://tracker.opentrackr.org:1337/announce"},{"udp://tracker.torrent.eu.org:451/announce"},{"https://tracker.gbitt.info/announce"}})
 select { case <-t.GotInfo(): case <-ctx.Done():fmt.Printf("BT NO METADATA peers=%d\\n",len(t.PeerConns()));return false }
 files:=t.Files()
 if len(files)==0 {return false}
 reader:=files[0].NewReader()
 defer reader.Close()
 data:=make([]byte,1)
 n,e:=reader.ReadContext(ctx,data)
 if n>0 {fmt.Printf("STREAM PASS hash=%s first_byte=%d peers=%d\\n",hash,data[0],len(t.PeerConns()));return true}
 fmt.Printf("STREAM FAIL hash=%s peers=%d error=%v\\n",hash,len(t.PeerConns()),e)
 return false
}
func main(){
 failures:=0
 streaming:=os.Getenv("VERIFY_BT_STREAM")=="1"
 for _,hash:=range hashes{
  fmt.Printf("START %s\\n",hash)
  ctx,cancel:=context.WithTimeout(context.Background(),150*time.Second)
  passed:=cache(ctx,hash)
  if !passed{passed=swarm(ctx,hash)}
  cancel()
  if passed && streaming {
    streamCtx,stop:=context.WithTimeout(context.Background(),90*time.Second)
    streamed:=streamFirstPiece(streamCtx,hash)
    stop()
    if !streamed { failures++; fmt.Printf("FAIL STREAM hash=%s\\n",hash)}
  }
  if !passed{failures++;fmt.Printf("FAIL hash=%s\\n",hash)}
 }
 if failures>0{fmt.Fprintf(os.Stderr,"%d magnet(s) could not be resolved\\n",failures);os.Exit(1)}
}
