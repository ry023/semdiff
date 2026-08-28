(function(){
var targets=new Map();
function key(anchor){return anchor.type+'\u0000'+anchor.group_id+'\u0000'+(anchor.fragment_id||'')}
function makeTarget(anchor,host,buttonHost,buttonClass){
  var panel=document.createElement('section');panel.className='qa-panel';panel.hidden=true;panel.dataset.qaKey=key(anchor);host.after(panel);
  var button=document.createElement('button');button.type='button';button.className='ask-button '+buttonClass;button.textContent='Ask';button.addEventListener('click',function(event){event.preventDefault();event.stopPropagation();openComposer(target)});buttonHost.append(button);
  var target={anchor:anchor,panel:panel,items:[]};targets.set(key(anchor),target);return target
}
function openComposer(target){
  target.panel.hidden=false;if(target.panel.querySelector('.qa-compose'))return;
  var form=document.createElement('form');form.className='qa-compose';form.innerHTML='<textarea required placeholder="Ask about this change…"></textarea><div class="qa-compose-actions"><button type="button" class="qa-cancel">Cancel</button><button type="submit" class="qa-submit">Submit</button></div><div class="qa-error" hidden></div>';
  target.panel.prepend(form);form.querySelector('.qa-cancel').addEventListener('click',function(){form.remove();if(!target.items.length)target.panel.hidden=true});form.addEventListener('submit',function(event){event.preventDefault();submit(target,form)});form.querySelector('textarea').focus()
}
async function submit(target,form){
  var textarea=form.querySelector('textarea'),errorNode=form.querySelector('.qa-error'),button=form.querySelector('.qa-submit');button.disabled=true;errorNode.hidden=true;
  try{var response=await fetch('/api/questions',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({anchor:target.anchor,question:textarea.value})});if(!response.ok)throw new Error(await response.text());form.remove();await refresh()}
  catch(error){errorNode.textContent=error.message||String(error);errorNode.hidden=false;button.disabled=false}
}
function render(target){
  target.panel.querySelectorAll('.qa-thread-list').forEach(function(node){node.remove()});if(!target.items.length){if(!target.panel.querySelector('.qa-compose'))target.panel.hidden=true;return}
  target.panel.hidden=false;var list=document.createElement('div');list.className='qa-thread-list';target.items.forEach(function(item){var thread=document.createElement('article');thread.className='qa-thread';var question=document.createElement('div');question.className='qa-question';var qLabel=document.createElement('span');qLabel.className='qa-label';qLabel.textContent='Q';question.append(qLabel,document.createTextNode(item.question));thread.append(question);if(item.status==='answered'){var answer=document.createElement('div');answer.className='qa-answer';var aLabel=document.createElement('span');aLabel.className='qa-label';aLabel.textContent='A';answer.append(aLabel,document.createTextNode(item.answer));thread.append(answer)}else{var status=document.createElement('div');status.className='qa-status';status.textContent=item.status==='claimed'?'Agent is working…':'Waiting for agent…';thread.append(status)}list.append(thread)});target.panel.append(list)
}
async function refresh(){
  try{var response=await fetch('/api/questions',{cache:'no-store'});if(!response.ok)return;var items=await response.json();targets.forEach(function(target){target.items=[]});items.forEach(function(item){var target=targets.get(key(item.anchor));if(target)target.items.push(item)});targets.forEach(render)}catch(error){}
}
document.querySelectorAll('.main-group').forEach(function(group){var summary=group.querySelector(':scope > summary');makeTarget({type:'group',group_id:group.dataset.groupId},summary,summary,'group-ask');group.querySelectorAll('.fragment-note').forEach(function(note){var id=(note.querySelector('.fragment-note-id')&&note.querySelector('.fragment-note-id').textContent.split(' · ')[0].trim())||'';if(!id)return;makeTarget({type:'fragment',group_id:group.dataset.groupId,fragment_id:id},note.closest('pre'),note,'fragment-ask')})});
refresh();setInterval(refresh,2000)
})();
