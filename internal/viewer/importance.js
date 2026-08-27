(function(){
var importanceDescriptions={
 core:'This group represents the reason the PR exists.',
 supporting:'This group completes, explains, or verifies the core change.',
 side:'This is a separately meaningful change bundled with the PR, but it is not needed to complete the core purpose.'
};
var reviewLevelDescriptions={
 careful:'Read closely; this change warrants extra reviewer attention.',
 normal:'Review at the usual level of attention.',
 skim:'A quick check should be sufficient; detailed review is not expected.'
};
var tooltip=document.createElement('div');tooltip.id='label-tooltip';tooltip.className='label-tooltip';tooltip.setAttribute('role','tooltip');tooltip.setAttribute('aria-hidden','true');document.body.append(tooltip);
function showTooltip(element){var description=element.dataset.tooltip;if(!description)return;tooltip.textContent=description;tooltip.classList.add('is-visible');tooltip.setAttribute('aria-hidden','false');var anchor=element.getBoundingClientRect();var box=tooltip.getBoundingClientRect();var top=anchor.top-box.height-8;if(top<8)top=anchor.bottom+8;var left=Math.max(8,Math.min(anchor.left+(anchor.width-box.width)/2,window.innerWidth-box.width-8));tooltip.style.top=Math.round(top)+'px';tooltip.style.left=Math.round(left)+'px'}
function hideTooltip(){tooltip.classList.remove('is-visible');tooltip.setAttribute('aria-hidden','true')}
function explain(element,value,descriptions){var description=descriptions[value];if(description&&!element.dataset.tooltipBound){element.dataset.tooltip=description;element.dataset.tooltipBound='true';element.setAttribute('aria-label',value+': '+description);element.tabIndex=0;element.addEventListener('mouseenter',function(){showTooltip(element)});element.addEventListener('mouseleave',hideTooltip);element.addEventListener('focus',function(){showTooltip(element)});element.addEventListener('blur',hideTooltip)}return element}
function importanceBadge(value,extra){var element=document.createElement('span');element.className='importance importance-'+value+(extra?' '+extra:'');element.textContent=value;return explain(element,value,importanceDescriptions)}
function reviewBadge(value,extra){var element=document.createElement('span');element.className='review-level review-level-'+value+(extra?' '+extra:'');element.textContent=value;return explain(element,value,reviewLevelDescriptions)}
function fragmentID(note){var id=note.querySelector('.fragment-note-id');return id?id.textContent.split(' · ')[0].trim():''}
function markBlock(note,value){note.classList.add('fragment-block','fragment-'+value,'main-fragment-'+value);var node=note.nextElementSibling;while(node&&!node.classList.contains('fragment-note')){node.classList.add('fragment-block','fragment-'+value,'main-fragment-'+value);node=node.nextElementSibling}}
document.querySelectorAll('.review-level').forEach(function(element){var value=Array.from(element.classList).find(function(name){return name.indexOf('review-level-')===0})||'';explain(element,value.slice('review-level-'.length),reviewLevelDescriptions)});
fetch('/importance.json').then(function(response){return response.json()}).then(function(data){
 document.querySelectorAll('.main-group[data-group-id]').forEach(function(group){var value=data.groups[group.dataset.groupId];if(!value)return;group.classList.add('group-'+value);var heading=group.querySelector(':scope > summary h2');if(heading)heading.after(importanceBadge(value,'group-importance'))});
 document.querySelectorAll('.nav-group > summary[data-group-id]').forEach(function(summary){var value=data.groups[summary.dataset.groupId];var title=summary.querySelector('.nav-group-title');if(value&&title)title.after(importanceBadge(value))});
 document.querySelectorAll('.file-fragment-description').forEach(function(description){var id=description.querySelector('.file-fragment-id');var value=id&&data.fragments[id.textContent.trim()];if(value){description.dataset.reviewLevel=value;description.prepend(reviewBadge(value,'fragment-review-level'))}});
 document.querySelectorAll('.fragment-note').forEach(function(note){var value=data.fragments[fragmentID(note)];if(value){note.dataset.reviewLevel=value;note.prepend(reviewBadge(value,'fragment-review-level'));markBlock(note,value)}});
}).catch(function(error){console.error('Failed to load importance data',error)});
})();
