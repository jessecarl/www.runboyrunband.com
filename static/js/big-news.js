$(document).ready(function(){
  /*** Helper function to set a cookie ***/
  setCookie = function(name, value, days) {
    var expires = '';
    var domain = '';

    if (days) {
      var date = new Date();
      date.setTime(date.getTime() + (days * 24 * 60 * 60 * 1000));
      expires = '; expires=' + date.toGMTString();
    }
    document.cookie = name + '=' + escape(value) + expires + '; path=/';
  };
  /*** Gets a cookie by name ***/
  getCookie = function(name) {
    var nameEQ = name + '=';
    var ca = document.cookie.split(';');

    for (var i = 0; i < ca.length; i++) {
      var c = ca[i];
      while (c.charAt(0) == ' ') {
        c = c.substring(1, c.length);
      }
      if (c.indexOf(nameEQ) == 0) {
        return unescape(c.substring(nameEQ.length, c.length));
      }
    }
    return "";
  };

  modalShown = false;
  $('.rbr-big-news').each(function(){
    if (modalShown) { return; }
    $this = $(this);
    if(getCookie($this.attr('id'))==="") {
      $this.modal();
      modalShown = true;
      $this.on('hidden.bs.modal', function(e){
        setCookie($this.attr('id'), true, 365); // unlikely to have a news item up for > 1 year
        $this.find('iframe').remove(); // to stop any running activity
      });
    }
  });
});
