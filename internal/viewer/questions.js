(function(){
var targets=new Map();
var collapsedThreads=new Set();
var questionsURL='/api/questions';
var answerMode=false;
var modeStyle=document.createElement('style');modeStyle.textContent='.qa-guide,.qa-mode-bar{margin:14px 0 20px;padding:10px 12px;border:1px solid var(--line);border-radius:7px;background:var(--panel)}.qa-guide{color:var(--muted)}.qa-guide summary{color:var(--text);font-weight:600;cursor:pointer}.qa-guide div{padding-top:9px;line-height:1.5}.qa-guide code{color:var(--accent)}.qa-mode-bar{display:flex;align-items:center;justify-content:space-between;color:var(--text)}.qa-mode-bar[hidden]{display:none}.qa-mode-bar button{padding:6px 10px;border:1px solid #8b3a3a;border-radius:5px;background:#3d2020;color:#ffb3b3;cursor:pointer}';document.head.append(modeStyle);
var guide=document.createElement('details');guide.className='qa-guide';guide.innerHTML='<summary>Ask AI about this review</summary><div>Ask your Agent to start the <code>answer-semdiff</code> skill. Ask buttons will appear while answer mode is active.</div>';var stats=document.querySelector('.stats');if(stats)stats.after(guide);
var modeBar=document.createElement('div');modeBar.className='qa-mode-bar';modeBar.hidden=true;modeBar.innerHTML='<span>AI answer mode is active</span><button type="button">End answer mode</button>';if(guide.parentNode)guide.after(modeBar);modeBar.querySelector('button').addEventListener('click',stopAnswerMode);
function key(anchor){return anchor.type+'\u0000'+anchor.group_id+'\u0000'+(anchor.step_id||'')+'\u0000'+(anchor.fragment_id||'')}
function makeTarget(anchor,host,buttonHost,buttonClass){
  var panel=document.createElement('section');panel.className='qa-panel';panel.hidden=true;panel.dataset.qaKey=key(anchor);host.after(panel);
  var button=document.createElement('button');button.type='button';button.className='ask-button '+buttonClass;button.textContent='Ask';button.addEventListener('click',function(event){event.preventDefault();event.stopPropagation();openComposer(target,null)});buttonHost.append(button);
  var target={anchor:anchor,panel:panel,button:button,threads:[]};button.hidden=!answerMode;targets.set(key(anchor),target);return target
}
function openComposer(target,threadID){
  target.panel.hidden=false;var selector=threadID?'[data-compose-thread="'+threadID+'"]':'[data-compose-thread="new"]';var existing=target.panel.querySelector(selector);if(existing){existing.remove();if(!target.threads.length)target.panel.hidden=true;return}
  var form=document.createElement('form');form.className='qa-compose';form.dataset.composeThread=threadID||'new';form.innerHTML='<textarea required placeholder="Ask about this change…"></textarea><div class="qa-compose-actions"><button type="button" class="qa-cancel">Cancel</button><button type="submit" class="qa-submit">Submit</button></div><div class="qa-error" hidden></div>';
  if(threadID)target.panel.append(form);else target.panel.prepend(form);
  form.querySelector('.qa-cancel').addEventListener('click',function(){form.remove();if(!target.threads.length)target.panel.hidden=true});form.addEventListener('submit',function(event){event.preventDefault();submit(target,form,threadID)});form.querySelector('textarea').focus()
}
async function submit(target,form,threadID){
  var textarea=form.querySelector('textarea'),errorNode=form.querySelector('.qa-error'),button=form.querySelector('.qa-submit');button.disabled=true;errorNode.hidden=true;
  var payload=threadID?{thread_id:threadID,question:textarea.value}:{anchor:target.anchor,question:textarea.value};
  try{var response=await fetch(questionsURL,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)});if(!response.ok)throw new Error(await response.text());form.remove();await refresh()}
  catch(error){errorNode.textContent=error.message||String(error);errorNode.hidden=false;button.disabled=false}
}
function textLine(className,label,content){var line=document.createElement('div');line.className=className;var marker=document.createElement('span');marker.className='qa-label';marker.textContent=label;line.append(marker,document.createTextNode(content));return line}
function render(target){
  target.panel.querySelectorAll('.qa-thread-list').forEach(function(node){node.remove()});if(!target.threads.length){if(!target.panel.querySelector('.qa-compose'))target.panel.hidden=true;return}
  target.panel.hidden=false;var list=document.createElement('div');list.className='qa-thread-list';target.threads.forEach(function(thread,index){
    var article=document.createElement('details');article.className='qa-thread';article.dataset.threadId=thread.id;article.open=!collapsedThreads.has(thread.id);article.addEventListener('toggle',function(){if(article.open)collapsedThreads.delete(thread.id);else collapsedThreads.add(thread.id)});var heading=document.createElement('summary');heading.className='qa-thread-summary';var first=thread.turns[0];heading.textContent='Thread '+(index+1)+(first?' · '+first.question:'');article.append(heading);var body=document.createElement('div');body.className='qa-thread-body';thread.turns.forEach(function(turn){var node=document.createElement('section');node.className='qa-turn';node.append(textLine('qa-question','Q',turn.question));if(turn.status==='answered')node.append(textLine('qa-answer','A',turn.answer));else{var status=document.createElement('div');status.className='qa-status';status.textContent=turn.status==='claimed'?'Agent is working…':'Waiting for agent…';node.append(status)}body.append(node)});
    var last=thread.turns[thread.turns.length-1];if(last&&last.status==='answered'&&answerMode){var actions=document.createElement('div');actions.className='qa-thread-actions';var follow=document.createElement('button');follow.type='button';follow.className='qa-follow-up';follow.textContent='Ask follow-up';follow.addEventListener('click',function(){openComposer(target,thread.id)});actions.append(follow);body.append(actions)}article.append(body);list.append(article)
  });target.panel.append(list)
}
async function refresh(){
  try{var responses=await Promise.all([fetch(questionsURL,{cache:'no-store'}),fetch(questionsURL+'/session',{cache:'no-store'})]);if(!responses[0].ok||!responses[1].ok)return;var threads=await responses[0].json();var session=await responses[1].json();answerMode=session.status==='active';guide.hidden=answerMode;modeBar.hidden=!answerMode;targets.forEach(function(target){target.threads=[];target.button.hidden=!answerMode;if(!answerMode)target.panel.querySelectorAll('.qa-compose').forEach(function(form){form.remove()})});threads.forEach(function(thread){var target=targets.get(key(thread.anchor));if(target)target.threads.push(thread)});targets.forEach(render)}catch(error){}
}
async function stopAnswerMode(){var button=modeBar.querySelector('button');button.disabled=true;try{var response=await fetch(questionsURL+'/session',{method:'POST'});if(!response.ok)throw new Error(await response.text());await refresh()}catch(error){window.alert(error.message||String(error))}finally{button.disabled=false}}
document.querySelectorAll('.guided-group').forEach(function(group){var summary=group.querySelector(':scope > summary');makeTarget({type:'group',group_id:group.dataset.groupId},summary,summary,'group-ask');group.querySelectorAll('.review-step').forEach(function(step){var stepSummary=step.querySelector(':scope > summary');makeTarget({type:'step',group_id:group.dataset.groupId,step_id:step.dataset.stepId},stepSummary,stepSummary,'step-ask');step.querySelectorAll('.fragment-note').forEach(function(note){var id=(note.querySelector('.fragment-note-id')&&note.querySelector('.fragment-note-id').textContent.split(' · ')[0].trim())||'';if(!id)return;makeTarget({type:'fragment',group_id:group.dataset.groupId,fragment_id:id},note.closest('.guided-file')||note.closest('pre'),note,'fragment-ask')})})});
refresh();setInterval(refresh,2000)
})();
