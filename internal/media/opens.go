package media

import "strings"

// Reading is what Polyfin found an input to be that FFmpeg or ffprobe
// reads on its loopback interface, before they read it: they are given
// the demuxers of that alone (see Opens).
type Reading int

const (
	// File is one file, or one endless stream.
	File Reading = iota
	// Playlist is an HLS playlist, which the loopback interface rewrites
	// so that every address it names is one of its own.
	Playlist
	// Manifest is a DASH manifest, rewritten likewise.
	Manifest
)

// loopback starts the addresses of Polyfin's loopback interface, which
// its port follows.
const loopback = "http://127.0.0.1:"

// oneFile are FFmpeg's demuxers that read the one file or stream they are
// given: every demuxer of FFmpeg 9.0.2 but those that open the addresses,
// files or ports the bytes they read name (hls, dash, concat, imf, sdp,
// rtsp, rtp, sap), those that read a sequence of files (image2,
// image2pipe), webm_dash_manifest, and those that run a script
// (avisynth, vapoursynth). The devices, which read no file, are not
// among them. vobsub and mlv stay: the other files they open are named
// after their own address, not by what they read.
const oneFile = "3dostr,3g2,3gp,4xm,aa,aac,aax,ac3,ac4,ace,acm,act,adf,adp,ads,adx,aea,afc,aiff,aix,alaw,alias_pix,alp,amr," +
	"amrnb,amrwb,anm,apac,apc,ape,apm,apng,aptx,aptx_hd,apv,aqtitle,argo_asf,argo_brp,argo_cvg,asf,asf_o,ass,ast," +
	"au,av1,avi,avr,avs,avs2,avs3,bethsoftvid,bfi,bfstm,bin,bink,binka,bit,bitpacked,bmp_pipe,bmv,boa,bonk," +
	"brender_pix,brstm,c93,caf,cavsvideo,cdg,cdxl,cine,codec2,codec2raw,cri_pipe,data,daud,dcstr,dds_pipe,derf," +
	"dfa,dfpwm,dhav,dirac,dnxhd,dpx_pipe,dsf,dsicin,dss,dts,dtshd,dv,dvbsub,dvbtxt,dxa,ea,ea_cdata,eac3,epaf,evc," +
	"exr_pipe,f32be,f32le,f64be,f64le,ffmetadata,film_cpk,filmstrip,fits,flac,flic,flv,frm,fsb,fwse,g722,g723_1," +
	"g726,g726le,g728,g729,gdv,gem_pipe,genh,gif,gif_pipe,gsm,gxf,h261,h263,h264,hca,hcom,hdr_pipe,hevc,hnm,hxvs," +
	"iamf,ico,idcin,idf,iff,ifv,ilbc,ingenient,ipmovie,ipu,ircam,iss,iv8,ivf,ivr,j2k_pipe,jacosub,jpeg_pipe," +
	"jpegls_pipe,jpegxl_anim,jpegxl_pipe,jpegxs_pipe,jv,kux,kvag,laf,lc3,libgme,libmodplug,libopenmpt,live_flv," +
	"lmlm4,loas,lrc,luodat,lvf,lxf,m4a,m4v,matroska,mca,mcc,mgsts,microdvd,mj2,mjpeg,mjpeg_2000,mlp,mlv,mm,mmf," +
	"mods,moflex,mov,mp3,mp4,mpc,mpc8,mpeg,mpegts,mpegtsraw,mpegvideo,mpjpeg,mpl2,mpsub,msf,msnwctcp,msp,mtaf," +
	"mtv,mulaw,musx,mv,mvi,mxf,mxg,nc,nistsphere,nsp,nsv,nut,nuv,obu,ogg,oma,osq,paf,pam_pipe,pbm_pipe,pcx_pipe," +
	"pdv,pfm_pipe,pgm_pipe,pgmyuv_pipe,pgx_pipe,phm_pipe,photocd_pipe,pictor_pipe,pjs,pmp,png_pipe,pp_bnk," +
	"ppm_pipe,psd_pipe,psxstr,pva,pvf,qcp,qdraw_pipe,qoa,qoi_pipe,r3d,rawvideo,rcwt,realtext,redspark,rka,rl2,rm," +
	"roq,rpl,rsd,rso,s16be,s16le,s24be,s24le,s32be,s32le,s337m,s8,sami,sbc,sbg,scc,scd,sdns,sdr2,sds,sdx,ser,sga," +
	"sgi_pipe,shn,siff,simbiosis_imx,sln,smjpeg,smk,smush,sol,sox,spdif,srt,stl,subviewer,subviewer1," +
	"sunrast_pipe,sup,svag,svg_pipe,svs,swf,tak,tedcaptions,thp,tiertexseq,tiff_pipe,tmv,truehd,tta,tty,txd,ty," +
	"u16be,u16le,u24be,u24le,u32be,u32le,u8,usm,v210,v210x,vag,vbn_pipe,vc1,vc1test,vidc,vividas,vivo,vmd,vobsub," +
	"voc,vpk,vplayer,vqf,vvc,w64,wady,wav,wavarc,wc3movie,webm,webp_anim,webp_pipe,webvtt,wsaud,wsd,wsvqa,wtv,wv," +
	"wve,xa,xbin,xbm_pipe,xmd,xmv,xpm_pipe,xvag,xwd_pipe,xwma,yop,yuv4mpegpipe"

// Opens are the options, before its -i, that name what FFmpeg or ffprobe
// may open for the input at address, found to be reads; none for an input
// that is not on Polyfin's loopback interface, a path or a disc Polyfin
// chose itself:
//
//   - the protocols the input needs, HTTP over TCP, and for a playlist
//     what its encrypted segments and inline keys are read with, so that
//     nothing it holds opens a file, a UDP port or a TLS connection;
//   - the demuxers of one file (see oneFile), which FFmpeg chooses among
//     by the bytes, and the playlist's or the manifest's demuxer only
//     where Polyfin found one, and rewrote it: the bytes of a file never
//     choose a demuxer that opens what they name;
//   - the loopback interface as the HTTP proxy, which refuses a request
//     for another host than its own: an address the rewriting left in a
//     playlist or a manifest is asked there, not fetched by FFmpeg.
//
// FFmpeg skips its proxy for the hosts the server's no_proxy environment
// variable names: for those the first two alone hold.
func Opens(address string, reads Reading) []string {
	rest, found := strings.CutPrefix(address, loopback)
	if !found {
		return nil
	}
	port, _, _ := strings.Cut(rest, "/")
	protocols, demuxers := "http,tcp", oneFile
	switch reads {
	case Playlist:
		protocols, demuxers = "http,tcp,crypto,data", "hls,"+oneFile
	case Manifest:
		demuxers = "dash," + oneFile
	}
	return []string{"-protocol_whitelist", protocols, "-format_whitelist", demuxers, "-http_proxy", loopback + port}
}
