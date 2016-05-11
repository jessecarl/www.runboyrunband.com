
/* Photo Carousel stuff */
$("#photo-carousel").on('slid.bs.carousel', function(){
  var $items = $(this).find('.item'),
  activeIndex = 0,
  i = 0;
  for( i = 0; i < $items.length; ++i ) {
    if ($($items[i]).hasClass('active')) {
      activeIndex = i;
    }
  }
  $('.photo-thumbs').each(function(){
    var $items = $(this).find('a.thumbnail').removeClass('active');
    $($items[activeIndex]).addClass('active');
  })
})
