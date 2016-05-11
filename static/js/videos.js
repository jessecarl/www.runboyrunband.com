$(document).ready(function(){
  var params = { allowScriptAccess: "always" };
  var atts = { id: "myytplayer" };
  var width = Math.min($('#ytapiplayer').width(),1920);
  var height = width * (9/16);
  swfobject.embedSWF("http://www.youtube.com/v/5ic7SkV4xeA?enablejsapi=1&version=3", "ytapiplayer", width, height, "8", null, null, params, atts);
});

function onYouTubePlayerReady(playerId) {
  ytplayer = document.getElementById("myytplayer");
  ytplayer.cuePlaylist({'listType': 'user_favorites', 'list':'runboyrunband', 'index':'0','startSeconds':'0','suggestedQuality':'hd1080'});
  $(window).resize(function(){
    var par = $(ytplayer).parent(),
    w = par.width(),
    h = w * (9/16);
    ytplayer.width = w;
    ytplayer.height = h;
  });
}

