(function(){
var threads=window.semdiffAnswers||[];
var targets=new Map();
function key(anchor){return anchor.type+'\u0000'+anchor.group_id+'\u0000'+(anchor.fragment_id||'')}
function textLine(className,label,content){var line=document.createElement('div');line.className=className;var marker=document.createElement('span');marker.className='qa-label';marker.textContent=label;line.append(marker,document.createTextNode(content));return line}
document.querySelectorAll('.main-group').forEach(function(group){
  targets.set(key({type:'group',group_id:group.dataset.groupId}),group.querySelector(':scope > summary'));
  group.querySelectorAll('.fragment-note').forEach(function(note){var id=(note.querySelector('.fragment-note-id')&&note.querySelector('.fragment-note-id').textContent.split(' · ')[0].trim())||'';if(id)targets.set(key({type:'fragment',group_id:group.dataset.groupId,fragment_id:id}),note.closest('pre'))})
});
var grouped=new Map();threads.forEach(function(thread){var id=key(thread.anchor);if(!grouped.has(id))grouped.set(id,[]);grouped.get(id).push(thread)});
grouped.forEach(function(items,id){var host=targets.get(id);if(!host)return;var panel=document.createElement('section');panel.className='qa-panel';var list=document.createElement('div');list.className='qa-thread-list';items.forEach(function(thread,index){var article=document.createElement('details');article.className='qa-thread';article.open=true;var heading=document.createElement('summary');heading.className='qa-thread-summary';heading.textContent='Thread '+(index+1)+' · '+thread.turns[0].question;article.append(heading);var body=document.createElement('div');body.className='qa-thread-body';thread.turns.forEach(function(turn){var node=document.createElement('section');node.className='qa-turn';node.append(textLine('qa-question','Q',turn.question),textLine('qa-answer','A',turn.answer));body.append(node)});article.append(body);list.append(article)});panel.append(list);host.after(panel)});
})();
