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
var reviewLevelIcons={
 careful:'<circle cx="12" cy="12" r="10"></circle><path d="M12 8v4"></path><path d="M12 16h.01"></path>',
 normal:'<circle cx="12" cy="12" r="10"></circle>',
 skim:'<path d="M10.1 2.182a10 10 0 0 1 3.8 0"></path><path d="M13.9 21.818a10 10 0 0 1-3.8 0"></path><path d="M17.609 3.72a10 10 0 0 1 2.69 2.7"></path><path d="M2.182 13.9a10 10 0 0 1 0-3.8"></path><path d="M20.28 17.61a10 10 0 0 1-2.7 2.69"></path><path d="M21.818 10.1a10 10 0 0 1 0 3.8"></path><path d="M3.721 6.391a10 10 0 0 1 2.7-2.69"></path><path d="M6.391 20.279a10 10 0 0 1-2.69-2.7"></path>'
};
var tooltip=document.createElement('div');tooltip.id='label-tooltip';tooltip.className='label-tooltip';tooltip.setAttribute('role','tooltip');tooltip.setAttribute('aria-hidden','true');document.body.append(tooltip);
function showTooltip(element){var description=element.dataset.tooltip;if(!description)return;tooltip.textContent=description;tooltip.classList.add('is-visible');tooltip.setAttribute('aria-hidden','false');var anchor=element.getBoundingClientRect();var box=tooltip.getBoundingClientRect();var top=anchor.top-box.height-8;if(top<8)top=anchor.bottom+8;var left=Math.max(8,Math.min(anchor.left+(anchor.width-box.width)/2,window.innerWidth-box.width-8));tooltip.style.top=Math.round(top)+'px';tooltip.style.left=Math.round(left)+'px'}
function hideTooltip(){tooltip.classList.remove('is-visible');tooltip.setAttribute('aria-hidden','true')}
function reviewIcon(value){var content=reviewLevelIcons[value];if(!content)return null;var icon=document.createElementNS('http://www.w3.org/2000/svg','svg');icon.setAttribute('viewBox','0 0 24 24');icon.setAttribute('fill','none');icon.setAttribute('stroke','currentColor');icon.setAttribute('stroke-width','2');icon.setAttribute('stroke-linecap','round');icon.setAttribute('stroke-linejoin','round');icon.setAttribute('aria-hidden','true');icon.innerHTML=content;return icon}
function explain(element,value,descriptions){var description=descriptions[value];if(description&&!element.dataset.tooltipBound){element.dataset.tooltip=description;element.dataset.tooltipBound='true';element.setAttribute('aria-label',value+': '+description);element.tabIndex=0;element.addEventListener('mouseenter',function(){showTooltip(element)});element.addEventListener('mouseleave',hideTooltip);element.addEventListener('focus',function(){showTooltip(element)});element.addEventListener('blur',hideTooltip)}return element}
function importanceBadge(value,extra){var element=document.createElement('span');element.className='importance importance-'+value+(extra?' '+extra:'');element.textContent=value;return explain(element,value,importanceDescriptions)}
function reviewBadge(value,extra){var element=document.createElement('span');element.className='review-level review-level-'+value+(extra?' '+extra:'');var icon=reviewIcon(value);if(icon)element.append(icon);else element.textContent=value;return explain(element,value,reviewLevelDescriptions)}
function fragmentID(note){var id=note.querySelector('.fragment-note-id');return id?id.textContent.split(' · ')[0].trim():''}
function markBlock(note,value){note.classList.add('fragment-block','fragment-'+value,'main-fragment-'+value);var node=note.nextElementSibling;while(node&&!node.classList.contains('fragment-note')){node.classList.add('fragment-block','fragment-'+value,'main-fragment-'+value);node=node.nextElementSibling}}
document.querySelectorAll('.review-level').forEach(function(element){var valueClass=Array.from(element.classList).find(function(name){return name.indexOf('review-level-')===0})||'';var value=valueClass.slice('review-level-'.length);var icon=reviewIcon(value);if(icon)element.replaceChildren(icon);explain(element,value,reviewLevelDescriptions)});
fetch('/importance.json').then(function(response){return response.json()}).then(function(data){
 document.querySelectorAll('.main-group[data-group-id]').forEach(function(group){var value=data.groups[group.dataset.groupId];if(!value)return;group.classList.add('group-'+value);var heading=group.querySelector(':scope > summary h2');if(heading)heading.after(importanceBadge(value,'group-importance'))});
 document.querySelectorAll('.nav-group > summary[data-group-id]').forEach(function(summary){var value=data.groups[summary.dataset.groupId];var title=summary.querySelector('.nav-group-title');if(value&&title)title.after(importanceBadge(value))});
 document.querySelectorAll('.file-fragment-description').forEach(function(description){var id=description.querySelector('.file-fragment-id');var value=id&&data.fragments[id.textContent.trim()];if(value){description.dataset.reviewLevel=value;description.prepend(reviewBadge(value,'fragment-review-level'))}});
 document.querySelectorAll('.fragment-note').forEach(function(note){var value=data.fragments[fragmentID(note)];if(value){note.dataset.reviewLevel=value;note.prepend(reviewBadge(value,'fragment-review-level'));markBlock(note,value)}});
}).catch(function(error){console.error('Failed to load importance data',error)});
})();
