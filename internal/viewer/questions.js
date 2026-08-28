(function(){
var targets=new Map();
function key(anchor){return anchor.type+'\u0000'+anchor.group_id+'\u0000'+(anchor.fragment_id||'')}
function makeTarget(anchor,host,buttonHost,buttonClass){
  var panel=document.createElement('section');panel.className='qa-panel';panel.hidden=true;panel.dataset.qaKey=key(anchor);host.after(panel);
  var button=document.createElement('button');button.type='button';button.className='ask-button '+buttonClass;button.textContent='Ask';button.addEventListener('click',function(event){event.preventDefault();event.stopPropagation();openComposer(target,null)});buttonHost.append(button);
  var target={anchor:anchor,panel:panel,threads:[]};targets.set(key(anchor),target);return target
}
function openComposer(target,threadID){
  target.panel.hidden=false;var selector=threadID?'[data-compose-thread="'+threadID+'"]':'[data-compose-thread="new"]';if(target.panel.querySelector(selector))return;
  var form=document.createElement('form');form.className='qa-compose';form.dataset.composeThread=threadID||'new';form.innerHTML='<textarea required placeholder="Ask about this change…"></textarea><div class="qa-compose-actions"><button type="button" class="qa-cancel">Cancel</button><button type="submit" class="qa-submit">Submit</button></div><div class="qa-error" hidden></div>';
  if(threadID)target.panel.append(form);else target.panel.prepend(form);
  form.querySelector('.qa-cancel').addEventListener('click',function(){form.remove();if(!target.threads.length)target.panel.hidden=true});form.addEventListener('submit',function(event){event.preventDefault();submit(target,form,threadID)});form.querySelector('textarea').focus()
}
async function submit(target,form,threadID){
  var textarea=form.querySelector('textarea'),errorNode=form.querySelector('.qa-error'),button=form.querySelector('.qa-submit');button.disabled=true;errorNode.hidden=true;
  var payload=threadID?{thread_id:threadID,question:textarea.value}:{anchor:target.anchor,question:textarea.value};
  try{var response=await fetch('/api/questions',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)});if(!response.ok)throw new Error(await response.text());form.remove();await refresh()}
  catch(error){errorNode.textContent=error.message||String(error);errorNode.hidden=false;button.disabled=false}
}
function textLine(className,label,content){var line=document.createElement('div');line.className=className;var marker=document.createElement('span');marker.className='qa-label';marker.textContent=label;line.append(marker,document.createTextNode(content));return line}
function render(target){
  target.panel.querySelectorAll('.qa-thread-list').forEach(function(node){node.remove()});if(!target.threads.length){if(!target.panel.querySelector('.qa-compose'))target.panel.hidden=true;return}
  target.panel.hidden=false;var list=document.createElement('div');list.className='qa-thread-list';target.threads.forEach(function(thread){
    var article=document.createElement('article');article.className='qa-thread';article.dataset.threadId=thread.id;thread.turns.forEach(function(turn){var node=document.createElement('section');node.className='qa-turn';node.append(textLine('qa-question','Q',turn.question));if(turn.status==='answered')node.append(textLine('qa-answer','A',turn.answer));else{var status=document.createElement('div');status.className='qa-status';status.textContent=turn.status==='claimed'?'Agent is working…':'Waiting for agent…';node.append(status)}article.append(node)});
    var last=thread.turns[thread.turns.length-1];if(last&&last.status==='answered'){var actions=document.createElement('div');actions.className='qa-thread-actions';var follow=document.createElement('button');follow.type='button';follow.className='qa-follow-up';follow.textContent='Ask follow-up';follow.addEventListener('click',function(){openComposer(target,thread.id)});actions.append(follow);article.append(actions)}list.append(article)
  });target.panel.append(list)
}
async function refresh(){
  try{var response=await fetch('/api/questions',{cache:'no-store'});if(!response.ok)return;var threads=await response.json();targets.forEach(function(target){target.threads=[]});threads.forEach(function(thread){var target=targets.get(key(thread.anchor));if(target)target.threads.push(thread)});targets.forEach(render)}catch(error){}
}
document.querySelectorAll('.main-group').forEach(function(group){var summary=group.querySelector(':scope > summary');makeTarget({type:'group',group_id:group.dataset.groupId},summary,summary,'group-ask');group.querySelectorAll('.fragment-note').forEach(function(note){var id=(note.querySelector('.fragment-note-id')&&note.querySelector('.fragment-note-id').textContent.split(' · ')[0].trim())||'';if(!id)return;makeTarget({type:'fragment',group_id:group.dataset.groupId,fragment_id:id},note.closest('pre'),note,'fragment-ask')})});
refresh();setInterval(refresh,2000)
})();
